// templates
package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
)

func responseError(w http.ResponseWriter, templateBody, e string) {
	templ, err := template.New("error").Parse(templateBody)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		fmt.Fprint(w, "Error:", err)
		return
	}

	type ErrorInfo struct{ ErrMsg string }

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = templ.ExecuteTemplate(w, "error", ErrorInfo{e})

	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		fmt.Fprint(w, "Error:", err)
		return
	}
}

func responseTemplate(w http.ResponseWriter, templateName, templateBody string, data interface{}) error {
	templ, err := template.New(templateName).Parse(templateBody)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	err = templ.ExecuteTemplate(&buf, templateName, data)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write(buf.Bytes()); err != nil {
		// Тут уже нельзя толкать в сокет, поскольку произошла ошибка при отсулке.
		// Поэтому просто показываем ошибку в логе сервера
		logError("responseFixedPage: ", err.Error())
	}
	return nil
}
