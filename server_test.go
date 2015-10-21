// server_test
package main

import (
	"bytes"
	"encoding/json"
	//"github.com/julienschmidt/httprouter"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func performRequest(t *testing.T, method, username, password, urlStr, body, response string, responseCode int) {
	req, _ := http.NewRequest(method, urlStr, bytes.NewReader([]byte(body)))
	if username != "" {
		req.SetBasicAuth(username, password)
	}
	if method == "POST" {
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Content-Length", strconv.Itoa(len(body)))
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

func TestServe(t *testing.T) {
	var data = url.Values{
		"ap": []string{"Тестовое%29 сообщение!!!"},
	}
	var tests = []struct {
		method       string
		urlStr       string
		username     string
		password     string
		body         string
		response     string
		responseCode int
	}{
		{"GET", "/Images/100.html", "", "", "", "100", http.StatusOK},
		{"GET", "/images/100.html", "", "", "", "100", http.StatusOK},
		{"GET", "/images/100.html?afsdfsfq4fwer", "", "", "", "100", http.StatusOK},
		{"GET", "/images/dir/100.html", "", "", "", "100", http.StatusOK},
		{"GET", "/images/dir/", "", "", "", "<pre><a href=\"100.html\">100.html</a></pre>", http.StatusOK},
		{"GET", "/", "", "", "", "<a href=\"/images\">Moved Permanently</a>.", http.StatusMovedPermanently},
		{"GET", "/ti8_a/a.server_test?ap=1", "", "", "", "1", http.StatusOK},
		{"GET", "/ti8_a/sfsfsf/a.server_test?ap=1", "", "", "", "1", http.StatusOK},
		{"POST", "/ti8/a.server_test", "a", "aaa111", data.Encode(), data.Get("ap"), http.StatusOK},
		{"POST", "/tI8/a.server_test", "a", "aaa111", data.Encode(), data.Get("ap"), http.StatusOK},
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

	for _, v := range tests {
		performRequest(t, v.method, v.username, v.password, v.urlStr, v.body, v.response, v.responseCode)
	}
}

func TestExpandFileName(t *testing.T) {
	resetConfig()
	var tests = []struct {
		srcStr string
		resStr string
	}{
		{"${APP_DIR}", basePath},
		{"${LOG_DIR}", basePath + "\\log\\"},
		{"${SERVICE_NAME}", serverconf.ServiceName},
		{"${DATE}", time.Now().Format("2006_01_02")},
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
	err = parseConfig(buf)
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range tests {
		res := expandFileName(v.srcStr)
		if res != v.resStr {
			t.Errorf("Response should be \"%s\", was \"%s\"", v.resStr, res)
		}
	}
}

type VD struct {
	Path               string `json:"Path"`
	Type               string `json:"Type"`
	RootDir            string `json:"RootDir"`
	RedirectPath       string `json:"RedirectPath"`
	SessionIdleTimeout int    `json:"owa.SessionIdleTimeout"`
	SessionWaitTimeout int    `json:"owa.SessionWaitTimeout"`
	RequestUserInfo    bool   `json:"owa.ReqUserInfo"`
	RequestUserRealm   string `json:"owa.ReqUserRealm"`
	DefUserName        string `json:"owa.DBUserName"`
	DefUserPass        string `json:"owa.DBUserPass"`
	BeforeScript       string `json:"owa.BeforeScript"`
	AfterScript        string `json:"owa.AfterScript"`
	ParamStoreProc     string `json:"owa.ParamStroreProc"`
	DocumentTable      string `json:"owa.DocumentTable"`
	Templates          []struct {
		Code string
		Body string
	} `json:"owa.Templates"`
	Grps []struct {
		ID  int32
		SID string
	} `json:"owa.UserGroups"`
}

var serverconf = struct {
	ServiceName      string `json:"Service.Name"`
	ServiceDispName  string `json:"Service.DisplayName"`
	HTTPPort         int    `json:"Http.Port"`
	HTTPDebugPort    int    `json:"Http.DebugPort"`
	HTTPReadTimeout  int    `json:"Http.ReadTimeout"`
	HTTPWriteTimeout int    `json:"Http.WriteTimeout"`
	HTTPSsl          bool   `json:"Http.SSL"`
	HTTPSslCert      string `json:"Http.SSLCert"`
	HTTPSslKey       string `json:"Http.SSLKey"`
	HTTPLogDir       string `json:"Http.LogDir"`
	HTTPUsers        []struct {
		Name      string
		IsSpecial bool
		GRP_ID    int32
	} `json:"Http.Users"`
	Handlers []VD `json:"Http.Handlers"`
}{

	ServiceName:      "ServiceName",
	ServiceDispName:  "ServiceDispName",
	HTTPPort:         9977,
	HTTPDebugPort:    8877,
	HTTPReadTimeout:  999,
	HTTPWriteTimeout: 888,
	HTTPSsl:          false,
	HTTPSslCert:      "HTTPSslCert",
	HTTPSslKey:       "HTTPSslKey",
	HTTPLogDir:       "${app_dir}\\log\\",
	HTTPUsers: []struct {
		Name      string
		IsSpecial bool
		GRP_ID    int32
	}{
		struct {
			Name      string
			IsSpecial bool
			GRP_ID    int32
		}{"A", false, 1},
		struct {
			Name      string
			IsSpecial bool
			GRP_ID    int32
		}{"USER001", false, 1},
	},
	Handlers: []VD{
		VD{
			Path:    "/Images",
			Type:    "Static",
			RootDir: "D:\\wwwroot\\Images\\",
		},
		VD{
			Path:         "/",
			Type:         "Redirect",
			RedirectPath: "/images",
		},
		VD{
			Path:               "/ti8",
			Type:               "owa_classic",
			SessionIdleTimeout: 30000,
			SessionWaitTimeout: 10000,
			RequestUserInfo:    true,
			RequestUserRealm:   "/ti8",
			DefUserName:        "",
			DefUserPass:        "",
			BeforeScript:       "session_init.init;",
			AfterScript:        "",
			ParamStoreProc:     "wex.ws",
			DocumentTable:      "wwv_document",
			Templates: []struct {
				Code string
				Body string
			}{
				{"error", "{{.ErrMsg}}"},
			},
			Grps: []struct {
				ID  int32
				SID string
			}{
				struct {
					ID  int32
					SID string
				}{1, "DP-TST8"},
			},
		},
		VD{
			Path:               "/ti8_a",
			Type:               "owa_classic",
			SessionIdleTimeout: 30000,
			SessionWaitTimeout: 10000,
			RequestUserInfo:    false,
			RequestUserRealm:   "/ti8",
			DefUserName:        "a",
			DefUserPass:        "aaa111",
			BeforeScript:       "session_init.init;",
			AfterScript:        "",
			ParamStoreProc:     "wex.ws",
			DocumentTable:      "wwv_document",
			Templates: []struct {
				Code string
				Body string
			}{
				{"error", "{{.ErrMsg}}"},
			},
			Grps: []struct {
				ID  int32
				SID string
			}{
				struct {
					ID  int32
					SID string
				}{1, "DP-TST8"},
			},
		},
	},
}
