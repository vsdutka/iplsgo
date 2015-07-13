// httpFileServer
package main

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"
	"sync"
)

type httpFileHandler struct {
	mu   sync.RWMutex
	path string
	root string
}

// HttpFileServer returns a handler that serves HTTP requests
// with the contents of the file system rooted at root.
//
// To use the operating system's file system implementation,
// use http.Dir:
//
//     http.Handle("/", http.FileServer(http.Dir("/tmp")))
func HttpFileServer(path, root string) HttpHandler {
	return &httpFileHandler{path: path, root: root}
}

func (f *httpFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/") {
		upath = "/" + upath
		r.URL.Path = upath
	}
	p, root := func() (string, string) {
		f.mu.RLock()
		defer f.mu.RUnlock()
		return f.path, f.root
	}()
	upath = strings.TrimPrefix(upath, p)
	http.ServeFile(w, r, path.Clean(root+upath))
}

func (f *httpFileHandler) SetConfig(conf *json.RawMessage) {
	type _t struct {
		RootDir string `json:"RootDir"`
	}
	t := _t{}
	if err := json.Unmarshal(*conf, &t); err != nil {
		logError(err)
	} else {
		func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.root = t.RootDir
		}()
	}
}
