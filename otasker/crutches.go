// crutches
package otasker

import (
	"bytes"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

func fixMeta(content []byte) []byte {
	buf := content
	for k := range bMeta {
		buf = bytes.Replace(buf, bMeta[k], bMetaEmpty, -1)
	}
	return buf
}

func fixContentType(contentType string) (string, string, bool) {
	mt, prms, err := mime.ParseMediaType(contentType)
	if err == nil {
		if strings.HasPrefix(mt, "text") ||
			strings.HasPrefix(mt, "application/json") ||
			strings.HasPrefix(mt, "application/x-sql") ||
			strings.HasPrefix(mt, "application/json") ||
			strings.HasPrefix(mt, "application/javascript") {
			charset, _ := prms["charset"]
			prms["charset"] = "utf-8"
			return mime.FormatMediaType(mt, prms), charset, true
		}
	}
	return contentType, "", false
}

func encodeFilename(afilename string) string {
	if strings.HasPrefix(afilename, "=?utf-8?") {
		return afilename
	}
	if s, err := url.QueryUnescape(afilename); err == nil {
		return url.QueryEscape(s)
	}
	return url.QueryEscape(afilename)
}

func parseHeaders(headers string) http.Header {
	fixedHeaders := make(http.Header)

	if headers != "" {
		for _, s := range strings.Split(headers, "\n") {
			if s != "" {
				i := strings.Index(s, ":")
				if i == -1 {
					i = len(s)
				}
				headerName := strings.TrimSpace(s[0:i])
				headerValue := ""
				if i < len(s) {
					headerValue = strings.TrimSpace(s[i+1:])
				}
				switch strings.ToLower(headerName) {
				case "content-disposition":
					{
						newVal := ""
						for _, partValue := range strings.Split(headerValue, "; ") {
							if strings.HasPrefix(partValue, "filename=") {
								newVal += "filename=\"" + encodeFilename(strings.Replace(strings.Replace(partValue, "filename=", "", -1), "\"", "", -1)) + "\";"
							} else {
								newVal += partValue + ";"
							}
						}
						fixedHeaders.Add(headerName, newVal)
					}
				default:
					{
						fixedHeaders.Add(headerName, headerValue)
					}
				}
			}
		}
	}
	return fixedHeaders
}

var (
	bMeta = [][]byte{
		[]byte(`<meta http-equiv="Content-Type" content="text/html; charset=windows-1251">`),
		[]byte(`<meta http-equiv=Content-Type content="text/html; charset=windows-1251">`),
		[]byte(`<meta http-equiv="CONTENT-TYPE content="text/html; charset=windows-1251">`),
		[]byte(`<meta http-equiv="Content-Type" content="text/html; charset=windows-1251">`),
	}
	bMetaEmpty = []byte(`<meta http-equiv="Content-Type" content="text/html; charset=utf-8">`)
)
