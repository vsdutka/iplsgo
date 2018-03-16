// logger
package main

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/vsdutka/metrics"
)

var countOfRequests = metrics.NewInt("Http_Number_Of_Requests", "HTTP - Number of http requests", "Items", "i")

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
				}
			}
		}
	}()
}
func writeToLog(msg string) {
	logChan <- msg
}
