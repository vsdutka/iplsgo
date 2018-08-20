// logger
package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vsdutka/metrics"
)

var countOfRequests = metrics.NewInt("Http_Number_Of_Requests", "HTTP - Number of http requests", "Items", "i")

type statusWriter struct {
	http.ResponseWriter
	status int
	length int
}

func (w *statusWriter) Hijack() (rwc net.Conn, buf *bufio.ReadWriter, err error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("Writer doesn't support hijacking")
	}
	return hj.Hijack()
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	w.length = len(b)
	return w.ResponseWriter.Write(b)
}

var logChan = make(chan string, 10000)

func init() {
	go func() {
		const fmtFileName = "${log_dir}/ex${date}.log"
		var (
			lastLogging = time.Time{}
			logFile     *os.File
		)
		defer func() {
			if logFile != nil {
				logFile.Close()
			}
		}()
		for {
			select {
			case str := <-logChan:
				{
					if lastLogging.Format("2006_01_02") != time.Now().Format("2006_01_02") {
						if logFile != nil {
							logFile.Close()
						}
						fileName := expandFileName(fmtFileName)
						dir, _ := filepath.Split(fileName)
						os.MkdirAll(dir, os.ModeDir)

						var err error

						logFile, err = os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
						if err != nil {
							logError(err)
							continue
						}
					}
					lastLogging = time.Now()
					if _, err := logFile.WriteString(str); err != nil {
						logError(err)
					}
					fmt.Print(str)
				}
			}
		}
	}()
}
func writeToLog(msg string) {
	logChan <- msg
}

type loggedHandler struct {
	handlerFunc func() http.Handler
}

func (l *loggedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.URL.Path = strings.ToLower(r.URL.Path)

	countOfRequests.Add(1)
	defer countOfRequests.Add(-1)

	start := time.Now()
	writer := statusWriter{w, 0, 0}
	handler := l.handlerFunc()
	handler.ServeHTTP(&writer, r)
	end := time.Now()
	latency := end.Sub(start)
	statusCode := writer.status
	length := writer.length
	user, _, ok := r.BasicAuth()
	if !ok {
		user = "-"
	}
	url := r.URL.Path

	params := r.Form.Encode()
	if params != "" {
		url = url + "?" + params
	}

	writeToLog(fmt.Sprintf("%s, %20s, %s, %s, %12d, %12d, %8d, %d, %s, %s, %v\r\n",
		r.RemoteAddr,
		user,
		end.Format("2006.01.02"),
		end.Format("15:04:05.000000000"),
		//r.Proto,
		//r.Host,
		length,
		r.ContentLength,
		time.Since(start)/time.Millisecond,
		statusCode,
		r.Method,
		url,
		latency,
	))
}
