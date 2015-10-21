// template_test
package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func execResponseError(t *testing.T, templateBody, message string, code int) {
	w := httptest.NewRecorder()
	responseError(w, templateBody, message)
	if w.Code != code {
		t.Fatalf("%s: got \"%v\",\nwant \"%v\"", "Code", w.Code, code)
	}
	if !bytes.Equal(w.Body.Bytes(), []byte(message)) {
		t.Fatalf("%s: got \"%v\",\nwant \"%v\"", "Message", w.Body.String(), message)
	}
}

func TestTemplateResponseError(t *testing.T) {
	var data = []struct {
		templateBody string
		message      string
		code         int
	}{
		{"{{.ErrMsg}}", "TestError", http.StatusOK},
		{"{{.ErrMsg}}", "Ошибка Ляляляля", http.StatusOK},
		{"{{.ErrMsg", "Error:template: error:1: unclosed action", http.StatusOK},
		{"{{.Err Msg}}", "Error:template: error:1: function \"Msg\" not defined", http.StatusOK},
		{"", "", http.StatusOK},
	}
	for _, v := range data {
		execResponseError(t, v.templateBody, v.message, v.code)
	}
}

func execResponseTemplate(t *testing.T, templateName, templateBody string, data interface{}, result string) {
	w := httptest.NewRecorder()
	wantBody := result
	wantCode := 200
	err := responseTemplate(w, templateName, templateBody, data)
	if err != nil {
		wantBody = result
		wantCode = 200
		execResponseError(t, "{{.ErrMsg}}", err.Error(), wantCode)
	}
	if w.Code != wantCode {
		t.Fatalf("%s: got \"%v\",\nwant \"%v\"", "Code", w.Code, wantCode)
	}
	if !bytes.Equal(w.Body.Bytes(), []byte(wantBody)) {
		t.Fatalf("%s: got \"%v\",\nwant \"%v\"", "Message", w.Body.String(), wantBody)
	}
}

func TestTemplateResponseTemplate(t *testing.T) {
	var data = []struct {
		templateName string
		templateBody string
		data         interface{}
		result       string
	}{
		{"TestTemplate", "{{.T.V}}", struct {
			T struct{ V string }
		}{
			T: struct {
				V string
			}{"Yes!"},
		},
			"Yes!"},
		{"TestTemplate", "{{range $key, $data := .T.V}}{{$key}}_{{$data}}{{end}}", struct {
			T struct{ V []string }
		}{
			T: struct {
				V []string
			}{[]string{"No!", "Yes!"}},
		},
			"0_No!1_Yes!"},
	}
	for _, v := range data {
		execResponseTemplate(t, v.templateName, v.templateBody, v.data, v.result)
	}
}
