// soap_test
package main

import (
	"bytes"
	"encoding/json"
	//"github.com/julienschmidt/httprouter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func performSoapRequest(t *testing.T, method, urlStr string, headers http.Header, body, response string, responseCode int) {
	req, _ := http.NewRequest(method, urlStr, bytes.NewReader([]byte(body)))

	if headers != nil {
		for k, v := range headers {
			for _, h := range v {
				req.Header.Add(k, h)
			}
		}
	}

	w := httptest.NewRecorder()

	serveHTTP(w, req)

	if w.Code != responseCode {
		t.Errorf("Method %s Status code should be %v, was %d", method, responseCode, w.Code)
	}
	res := strings.Replace(w.Body.String(), "\n", "", -1)
	if res != response {
		t.Errorf("Method %s Response should be \"%s\", was \"%s\"", method, response, res)
	}
}

func TestSoap(t *testing.T) {
	var tests = []struct {
		method       string
		urlStr       string
		headers      http.Header
		body         string
		response     string
		responseCode int
	}{
		{"GET", "/soap", nil, "", "<a href=\"/soap/\">Moved Permanently</a>.", http.StatusMovedPermanently},
		{"POST", "/soap", nil, "", "", http.StatusTemporaryRedirect},
		{"GET", "/soap/soap", nil, "", "soap: Body required", http.StatusBadRequest},
		{"GET", "/soap/soap?WSDL", nil, "", "WSDL", http.StatusOK},
		{"POST", "/soap/soap", nil, "", "soap: Body required", http.StatusBadRequest},
		{"POST", "/soap/soap", http.Header{"SOAPAction": []string{"ActionGet"}}, "", "soap: Body required", http.StatusBadRequest},
		{"POST", "/soap/soap", http.Header{"SOAPAction": []string{"ActionGet"}}, "BODY", "BODY", http.StatusOK},
	}

	buf, err := json.Marshal(serverconf)
	if err != nil {
		t.Fatal(err)
	}
	resetConfig()
	err = parseConfig(buf)
	if err != nil {
		t.Fatal(err)
	}
	err = exec(dsn, `create or replace function soap(ABody in clob) return clob 
is
begin
  if ABody = 'WSDL' then
    return 'WSDL';
  else
    Return 'BODY';
  end if;
end;
`)
	if err != nil {
		t.Fatalf("Error when create function \"soap\": %s", err.Error())
	}

	for _, v := range tests {
		performSoapRequest(t, v.method, v.urlStr, v.headers, v.body, v.response, v.responseCode)
	}

	err = exec(dsn, "drop function soap")
	if err != nil {
		t.Fatalf("Error when drop function \"soap\": %s", err.Error())
	}

}
