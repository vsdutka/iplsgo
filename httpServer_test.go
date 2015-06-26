// httpServer_test
package main

import (
	//"io/ioutil"
	"net/http"
	"net/http/httptest"
	//"os"
	//"path"
	//"strings"
	"crypto/rand"
	"testing"
)

func PerformRequest(r http.Handler, method, path string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func testMuxOK(t *testing.T, mux *HttpServeMux, method, url string) {
	w := PerformRequest(mux, method, url)
	if w.Code != http.StatusOK {
		t.Errorf("Method %s Status code should be %v, was %d", method, http.StatusOK, w.Code)
	}
	res := w.Body.String()
	if res != url {
		t.Errorf("Method %s Response should be \"%s\", was \"%s\"", method, url, res)
	}
}

func testMuxNotOK(t *testing.T, mux *HttpServeMux, method, url string) {
	w := PerformRequest(mux, method, url)
	if w.Code != http.StatusNotFound {
		t.Errorf("Method %s Status code should be %v, was %d", method, http.StatusNotFound, w.Code)
	}
	res := w.Body.String()
	if res == url {
		t.Errorf("Method %s Response should be \"%s\", was \"%s\"", method, "404 page not found", res)
	}
}

var methods = []string{
	"GET",
	"POST",
	"DELETE",
	"PATCH",
	"PUT",
	"OPTIONS",
	"HEAD",
}

func testPathMatchOk(t *testing.T, pattern, url string) {
	if !pathMatch(pattern, url) {
		t.Errorf("Path %s should be mached to %v", url, pattern)
	}
}
func testPathMatchNotOk(t *testing.T, pattern, url string) {
	if pathMatch(pattern, url) {
		t.Errorf("Path %s should not be mached to %v", url, pattern)
	}
}

func TestMuxOK(t *testing.T) {
	var urls = []string{
		"/test1",
		"/test2/test1",
		"/test3/test1/test2",
	}

	for _, v := range urls {
		testPathMatchOk(t, v+"/", v+"/1")
		testPathMatchNotOk(t, v+"/", v+"_2/1")
		testPathMatchNotOk(t, v, v+"_2/1")
	}

	r := NewHttpServeMux()

	for _, v := range urls {
		r.Handle(v+"/", &echoHttpHandler{})
	}

	for _, v := range urls {
		for _, m := range methods {
			testMuxOK(t, r, m, v+"/1")
		}
	}
	for _, v := range urls {
		for _, m := range methods {
			testMuxNotOK(t, r, m, v+"_2/1")
		}
	}
}

func TestMuxDelete(t *testing.T) {
	var urls = []string{
		"/test1",
		"/test2/test1",
		"/test3/test1/test2",
	}

	r := NewHttpServeMux()

	for _, v := range urls {
		r.Handle(v+"/", &echoHttpHandler{})
	}

	testMuxOK(t, r, "GET", "/test2/test1/1")
	r.Delete("/test2/test1/")
	testMuxNotOK(t, r, "GET", "/test2/test1/1")
}

func BenchmarkAppendToMux(b *testing.B) {
	r := NewHttpServeMux()
	for i := 0; i < b.N; i++ {
		bt := make([]byte, 40)
		_, err := rand.Read(bt)
		if err != nil {
			b.Error(err)
			return
		}
		r.Handle(string(bt), &echoHttpHandler{})
	}
}
