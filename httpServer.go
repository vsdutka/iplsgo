// httpServer
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
)

// HttpHandler расширяет http.Handler
type HttpHandler interface {
	http.Handler
	SetConfig(conf *json.RawMessage)
}

// HttpServeMux заменяет http.ServeMux
type HttpServeMux struct {
	mu      sync.RWMutex
	keys    []string // Список путей
	entries map[string]muxEntry
	hosts   bool // whether any patterns contain hostnames
}

type muxEntry struct {
	explicit bool
	h        HttpHandler
	pattern  string
}

// NewHttpServeMux allocates and returns a new ServeMux.
func NewHttpServeMux() *HttpServeMux {
	return &HttpServeMux{keys: make([]string, 100), entries: make(map[string]muxEntry)}
}

// Does path match pattern?
func pathMatch(pattern, path string) bool {
	if len(pattern) == 0 {
		// should not happen
		return false
	}
	n := len(pattern)
	if pattern[n-1] != '/' {
		return pattern == path
	}
	return len(path) >= n && path[0:n] == pattern
}

// Return the canonical path for p, eliminating . and .. elements.
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	np := path.Clean(p)
	// path.Clean removes trailing slash except for root;
	// put the trailing slash back if necessary.
	if p[len(p)-1] == '/' && np != "/" {
		np += "/"
	}
	return np
}

// Find a handler on a handler map given a path string
// Most-specific (longest) pattern wins
func (mux *HttpServeMux) match(path string) (h HttpHandler, pattern string) {
	path = strings.ToLower(path)

	var n = 0
	for _, v := range mux.keys {
		if !pathMatch(v, path) {
			continue
		}
		if h == nil || len(v) > n {
			n = len(v)
			h = mux.entries[v].h
			pattern = mux.entries[v].pattern
		}
	}
	return
}

// Handler returns the handler to use for the given request,
// consulting r.Method, r.Host, and r.URL.Path. It always returns
// a non-nil handler. If the path is not in its canonical form, the
// handler will be an internally-generated handler that redirects
// to the canonical path.
//
// Handler also returns the registered pattern that matches the
// request or, in the case of internally-generated redirects,
// the pattern that will match after following the redirect.
//
// If there is no registered handler that applies to the request,
// Handler returns a ``page not found'' handler and an empty pattern.
func (mux *HttpServeMux) Handler(r *http.Request) (h HttpHandler, pattern string) {
	if r.Method != "CONNECT" {
		if p := cleanPath(r.URL.Path); p != r.URL.Path {
			_, pattern = mux.handler(r.Host, p)
			url := *r.URL
			url.Path = p
			return RedirectHandler(url.String(), http.StatusMovedPermanently), pattern
		}
	}

	return mux.handler(r.Host, r.URL.Path)
}

// handler is the main implementation of Handler.
// The path is known to be in canonical form, except for CONNECT methods.
func (mux *HttpServeMux) handler(host, path string) (h HttpHandler, pattern string) {
	mux.mu.RLock()
	defer mux.mu.RUnlock()

	// Host-specific pattern takes precedence over generic ones
	if mux.hosts {
		h, pattern = mux.match(host + path)
	}
	if h == nil {
		h, pattern = mux.match(path)
	}
	if h == nil {
		h, pattern = NotFoundHandler(), ""
	}
	return
}

// ServeHTTP dispatches the request to the handler whose
// pattern most closely matches the request URL.
func (mux *HttpServeMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.RequestURI == "*" {
		if r.ProtoAtLeast(1, 1) {
			w.Header().Set("Connection", "close")
		}
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	h, _ := mux.Handler(r)
	h.ServeHTTP(w, r)
}

// Handle registers the handler for the given pattern.
// If a handler already exists for pattern, Handle panics.
func (mux *HttpServeMux) Handle(pattern string, handler HttpHandler) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	pattern = strings.ToLower(pattern)

	if pattern == "" {
		panic("http: invalid pattern " + pattern)
	}
	if handler == nil {
		panic("http: nil handler")
	}
	if mux.entries[pattern].explicit {
		panic("http: multiple registrations for " + pattern)
	}

	mux.entries[pattern] = muxEntry{explicit: true, h: handler, pattern: pattern}
	pk := append(mux.keys, pattern)

	if pattern[0] != '/' {
		mux.hosts = true
	}

	// Helpful behavior:
	// If pattern is /tree/, insert an implicit permanent redirect for /tree.
	// It can be overridden by an explicit registration.
	n := len(pattern)
	if n > 0 && pattern[n-1] == '/' && !mux.entries[pattern[0:n-1]].explicit {
		// If pattern contains a host name, strip it and use remaining
		// path for redirect.
		path := pattern
		if pattern[0] != '/' {
			// In pattern, at least the last character is a '/', so
			// strings.Index can't be -1.
			path = pattern[strings.Index(pattern, "/"):]
		}
		mux.entries[pattern[0:n-1]] = muxEntry{h: RedirectHandler(path, http.StatusMovedPermanently), pattern: pattern}
		pk = append(pk, path)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(pk)))
	mux.keys = pk
}

// Delete удаляет Handler из списка обрабатываемых
func (mux *HttpServeMux) Delete(pattern string) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	pattern = strings.ToLower(pattern)

	if pattern == "" {
		panic("http: invalid pattern " + pattern)
	}

	patternRedirect := ""

	n := len(pattern)
	if n > 0 && pattern[n-1] == '/' && !mux.entries[pattern[0:n-1]].explicit {
		// If pattern contains a host name, strip it and use remaining
		// path for redirect.
		patternRedirect = pattern
		if pattern[0] != '/' {
			// In pattern, at least the last character is a '/', so
			// strings.Index can't be -1.
			patternRedirect = pattern[strings.Index(pattern, "/"):]
		}
	}

	var (
		keysDeleted []string
		keysResult  []string
	)
	for _, v := range mux.keys {
		if (v == pattern) || (v == patternRedirect) {
			keysDeleted = append(keysDeleted, v)
		} else {
			keysResult = append(keysResult, v)
		}
	}

	mux.keys = keysResult
	for _, v := range keysDeleted {
		delete(mux.entries, v)
	}
}

// GetHandler возвращает HttpHandler по пути
func (mux *HttpServeMux) GetHandler(pattern string) (HttpHandler, bool) {
	mux.mu.RLock()
	defer mux.mu.RUnlock()

	pattern = strings.ToLower(pattern)

	if pattern == "" {
		return nil, false
	}

	e, ok := mux.entries[pattern]
	if !ok {
		return nil, false
	}
	return e.h, true
}

// Redirect to a fixed URL
type redirectHandler struct {
	mu   sync.RWMutex
	url  string
	code int
}

func (rh *redirectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	url, code := func() (string, int) {
		rh.mu.RLock()
		defer rh.mu.RUnlock()
		return rh.url, rh.code
	}()
	http.Redirect(w, r, url, code)
}

func (rh *redirectHandler) SetConfig(conf *json.RawMessage) {
	type _t struct {
		RedirectPath string `json:"RedirectPath"`
	}
	t := _t{}
	if err := json.Unmarshal(*conf, &t); err != nil {
		fmt.Println(err)
	} else {
		func() {
			rh.mu.Lock()
			defer rh.mu.Unlock()
			rh.url = t.RedirectPath
		}()
	}
}

// RedirectHandler returns a request handler that redirects
// each request it receives to the given url using the given
// status code.
func RedirectHandler(url string, code int) HttpHandler {
	return &redirectHandler{url: url, code: code}
}

// Error replies to the request with the specified error message and HTTP code.
// The error message should be plain text.
func Error(w http.ResponseWriter, error string, code int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintln(w, error)
}

type errortHandler struct {
	error string
	code  int
}

func (eh *errortHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	Error(w, eh.error, eh.code)
}

func (eh *errortHandler) SetConfig(conf *json.RawMessage) {

}

// NotFoundHandler returns a simple request handler
// that replies to each request with a ``404 page not found'' reply.404 page not found
func NotFoundHandler() HttpHandler { return &errortHandler{"404 page not found", http.StatusNotFound} }

type echoHttpHandler struct{}

func (h *echoHttpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(r.URL.Path))
}

func (h *echoHttpHandler) SetConfig(conf *json.RawMessage) {
}
