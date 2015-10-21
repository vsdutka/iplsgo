// cgienvparams
package main

import (
	"net/http"
	"strings"
)

func makeEnvParams(req *http.Request, docTab, remoteUser, mirrorPath string) map[string]string {
	https := "N"
	portSequre := "0"

	if req.TLS != nil {
		https = "Y"
		portSequre = "1"
	}

	host := ""
	port := ""

	for k, v := range strings.Split(req.Host, ":") {
		switch {
		case k == 0:
			host = v
		case k == 1:
			port = v
		}
	}

	return map[string]string{
		"SERVER_SOFTWARE":      "iPLSQL",
		"SERVER_NAME":          host,
		"GATEWAY_INTERFACE":    "CGI/1.1",
		"REMOTE_HOST":          req.RemoteAddr,
		"REMOTE_ADDR":          req.RemoteAddr,
		"AUTH_TYPE":            req.Header.Get("Authorization"),
		"REMOTE_USER":          remoteUser,
		"REMOTE_IDENT":         remoteUser,
		"HTTP_ACCEPT":          req.Header.Get("Accept"),
		"HTTP_USER_AGENT":      req.Header.Get("User-Agent"),
		"SERVER_PROTOCOL":      req.Proto,
		"SERVER_PORT":          port,
		"SCRIPT_NAME":          "",
		"PATH_INFO":            req.URL.Path,
		"PATH_TRANSLATED":      "",
		"HTTP_REFERER":         req.Header.Get("Referer"),
		"HTTP_COOKIE":          req.Header.Get("Cookie"),
		"HTTP_ACCEPT_ENCODING": req.Header.Get("Accept-Encoding"),
		"HTTP_ACCEPT_CHARSET":  req.Header.Get("Accept-Charset"),
		"HTTP_ACCEPT_LANGUAGE": req.Header.Get("Accept-Language"),
		"PLSQL_GATEWAY":        "WebDb",
		"GATEWAY_IVERSION":     "3",
		"DOCUMENT_TABLE":       docTab,
		"QUERY_STRING":         req.URL.RawQuery,
		"HTTPS":                https,
		"SERVER_PORT_SECURE":   portSequre,
		"HTTPS_SESSIONID":      req.Header.Get("HTTPS_SESSIONID"),
		"HTTPS_KEYSIZE":        req.Header.Get("HTTPS_KEYSIZE"),
		"HTTPS_SERVER_ISSUER":  req.Header.Get("HTTPS_SERVER_ISSUER"),
		"HTTPS_SERVER_SUBJECT": req.Header.Get("HTTPS_SERVER_SUBJECT"),
		"cookie":               req.Header.Get("Cookie"),
		"user-agent":           req.Header.Get("User-Agent"),
		"referer":              req.Header.Get("Referer"),
		"accept":               req.Header.Get("Accept"),
		"accept-encoding":      req.Header.Get("Accept-Encoding"),
		"accept-language":      req.Header.Get("Accept-Language"),
		"pragma":               req.Header.Get("pragma"),
		"REQUEST_CHARSET":      "AL32UTF8",
		"REQUEST_IANA_CHARSET": "",
		"REQUEST_METHOD":       req.Method,
		"REQUEST_PROTOCOL":     req.Proto,
		"REQUEST_SCHEME":       req.Proto,
		"AUTHORIZATION":        req.Header.Get("Authorization"),
		"MIRROR_PATH":          mirrorPath,
	}
}
