// commonUtils_test
package main

import (
	"bytes"

	"fmt"

	"net/http"
	//"net/http/httptest"
	//"net/url"

	"html/template"
	"testing"

	"github.com/vsdutka/mltpart"
)

func TestMakeWaitForm(t *testing.T) {
	// Create a request to pass to our handler. We don't have any query parameters for now, so we'll
	// pass 'nil' as the third parameter.
	req, err := http.NewRequest("POST", "https://dp-asw3:11111/asrolf-ti10/azag$acu_ag.actionquery",
		bytes.NewBufferString("P_L_PT_DC_D=%D0%A0%D0%9E%D0%9B%D0%AC%D0%A4+%D0%94%D0%B8%D0%B0%D0%BC%D0%B0%D0%BD%D1%82&P_L_AGT_D=%D0%A1%D1%87%D0%B5%D1%82-%D0%97%D0%B0%D0%BA%D0%B0%D0%B7&P_L_AGRST_N_R=%D0%97%D0%B0%D0%B2%D0%B5%D1%80%D1%88%D0%B5%D0%BD%D0%BE&P_HID=01%2F01%2F2015&P_SGND=&U_SGND=&P_BEGD=&U_BEGD=&P_FIND=&U_FIND=&P_COMMS=&P_L_WARRCLAIM_NUM=&Z_ACTION=%D0%9F%D0%BE%D0%B8%D1%81%D0%BA&Z_CHK=0"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	_, _ = mltpart.ParseMultipartFormEx(req, 64<<20)

	fmt.Println(req.Form)
	s := makeWaitForm(req, "999")
	fmt.Println(s)

	fmt.Println(template.HTML(s))

	//	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	//	rr := httptest.NewRecorder()
	//	handler := http.HandlerFunc(HealthCheckHandler)

	//	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	//	// directly and pass in our Request and ResponseRecorder.
	//	handler.ServeHTTP(rr, req)

	//	// Check the status code is what we expect.
	//	if status := rr.Code; status != http.StatusOK {
	//		t.Errorf("handler returned wrong status code: got %v want %v",
	//			status, http.StatusOK)
	//	}

	//	// Check the response body is what we expect.
	//	expected := `{"alive": true}`
	//	if rr.Body.String() != expected {
	//		t.Errorf("handler returned unexpected body: got %v want %v",
	//			rr.Body.String(), expected)
	//	}
}
