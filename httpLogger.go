// httpLogger
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
	length int
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

// WriteLog Logs the Http Status for a request into fileHandler and returns a httphandler function which is a wrapper to log the requests.
func WriteLog(handle http.Handler, s *applicationServer) http.HandlerFunc {
	logChan := make(chan string, 10000)
	go func() {
		const fmtFileName = "${log_dir}\\ex${date}.log"
		var (
			lastLogging time.Time = time.Time{}
			logFile     *os.File
			err         error
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
						fileName := s.expandFileName(fmtFileName)
						s.checkDirExists(fileName)
						logFile, err = os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
						if err != nil {
							log.Fatalln(err)
						}
					}
					lastLogging = time.Now()
					logFile.WriteString(str)
				}
			}
		}
	}()
	return func(w http.ResponseWriter, request *http.Request) {
		start := time.Now()
		writer := statusWriter{w, 0, 0}
		handle.ServeHTTP(&writer, request)
		end := time.Now()
		latency := end.Sub(start)
		statusCode := writer.status
		length := writer.length
		user, _, ok := request.BasicAuth()
		if !ok {
			user = "-"
		}
		//"%s, %s, %02d.%02d.%02d, %02d:%02d:%02d, %s, %s, %d, %d, %d, %d, %s, %s, %v\n"
		url := request.RequestURI

		params := request.Form.Encode()
		if params != "" {
			url = url + "?" + params
		}

		logChan <- fmt.Sprintf("%s, %s, %s, %s, %s, %s, %d, %d, %d, %d, %s, %s, %v\n",
			request.RemoteAddr,
			user,
			end.Format("2006.01.02"),
			end.Format("15:04:05.000000000"),
			//			end.Day(),
			//			end.Hour(),
			//			end.Minute(),
			//			end.Second(),
			request.Proto,
			request.Host,
			length,
			request.ContentLength,
			time.Since(start)/time.Millisecond,
			statusCode,
			request.Method,
			url,
			latency,
		)

		//		if request.URL.RawQuery != "" {
		//			logChan <- fmt.Sprintf("%v %s %s \"%s %s%s%s %s\" %d %d \"%s\" %v", end.Format("2006/01/02 15:04:05"), request.Host, request.RemoteAddr, request.Method, request.URL.Path, "?", request.URL.RawQuery, request.Proto, statusCode, length, request.Header.Get("User-Agent"), latency)
		//		} else {
		//			logChan <- fmt.Sprintf("%v %s %s \"%s %s %s\" %d %d \"%s\" %v", end.Format("2006/01/02 15:04:05"), request.Host, request.RemoteAddr, request.Method, request.URL.Path, request.Proto, statusCode, length, request.Header.Get("User-Agent"), latency)
		//		}
	}
}
