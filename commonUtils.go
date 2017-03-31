// commonUtils
package main

import (
	"net"
	"net/http"
	//"net/url"
	"strings"

	"github.com/pborman/uuid"
	"github.com/vsdutka/iplsgo/otasker"
)

func makeHandlerID(isSpecial bool, userName, userPass, debugIP string, req *http.Request) string {
	addr := ""
	if isSpecial {
		addr = req.Header.Get("X-Real-IP")
		if addr == "" {
			addr = req.Header.Get("X-Forwarded-For")
			if addr == "" {
				addr = req.RemoteAddr
			}
		}
	}
	host, _, _ := net.SplitHostPort(addr)
	return strings.ToUpper(userName + "|" + userPass + "|" + host + "|" + debugIP)
}

func makeTaskID(req *http.Request) string {
	mID := req.FormValue("MessageId")
	if mID == "" {
		mID = uuid.New()
	}
	return mID
}

func makeWaitForm(req *http.Request, taskID string) string {
	s := req.URL.Path
	if req.URL.RawQuery != "" {
		s = s + "?" + req.URL.RawQuery
	}
	s = "<form id=\"__gmrf__\" action=\"" + s + "\" method=\"post\" >\n"
	for key, vals := range req.PostForm {
		for val := range vals {
			s = s + "<input type=\"hidden\" name=\"" + key + "\" value=\"" + /*url.QueryEscape(vals[val])*/ strings.Replace(vals[val], "\"", "&quot;", -1) + "\">\n"
		}
	}
	if req.MultipartForm != nil {
		for key, vals := range req.MultipartForm.File {
			for _, fileHeader := range vals {
				//_, fileName := filepath.Split(fileHeader.Filename)
				fileName := otasker.ExtractFileName(fileHeader.Header.Get("Content-Disposition"))
				s = s + "<input type=\"hidden\" name=\"" + key + "\" value=\"" + fileName + "\">\n"
			}
		}
	}
	if req.FormValue("MessageId") == "" {
		s = s + "<input type=\"hidden\" name=\"MessageId\" value=\"" + taskID + "\">\n"
	}
	s = s + "</form>"
	return s
}
