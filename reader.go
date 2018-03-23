// config
package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/vsdutka/iplsgo/otasker"
	"github.com/vsdutka/metrics"
	errgo "gopkg.in/errgo.v1"
	"gopkg.in/goracle.v1/oracle"
)

var (
	configReadDuration = metrics.NewFloat("config_read_duration", "Config - Read duration", "Seconds", "s")
	readerLog          *log.Logger
)

const fmtReaderFileName = "./reader.log"

var (
	stopChan        = make(chan struct{})
	stoppedChan     = make(chan struct{})
	conn            *oracle.Connection
	reader_username string
	reader_password string
	reader_sid      string
	configname      string
	hostname        string
)

func initReading(dsn, configName string) error {
	reader_username, reader_password, reader_sid = oracle.SplitDSN(dsn)
	configname = configName
	var err error
	hostname = *hostFlag
	if hostname == "" {
		if hostname, err = os.Hostname(); err != nil {
			return errgo.Newf("Error getting host name: %s\n", err)
		}
	}
	//В этот момент нет конфигурации, поэтому создаем файл лога рядом с исполнимым файлом
	dir, _ := filepath.Split(fmtReaderFileName)
	os.MkdirAll(dir, os.ModeDir)
	logFile, err := os.OpenFile(fmtReaderFileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println("Error = ", err)
		logError(err)
	}
	readerLog = log.New(logFile, "", log.LstdFlags|log.Lshortfile)
	return nil
}

func startReading(dsn, configName string, timeout time.Duration) error {

	var (
		err error
		buf []byte
	)
	if err = initReading(dsn, configName); err != nil {
		return err
	}

	if buf, err = readConfig(); err != nil {
		return errgo.Newf("Error read configuration: %s\n", err)
	}
	if err = parseConfig(buf); err != nil {
		return errgo.Newf("Error parse configuration: %s\n", err)
	}
	go func(timeout time.Duration) {
		defer func() {
			if conn != nil {
				if conn.IsConnected() {
					conn.Close()
				}
			}
		}()

		timer := time.NewTimer(timeout)

		for {
			select {
			case <-stopChan:
				{
					conn.Free(true)
					conn = nil
					stoppedChan <- struct{}{}
					return
				}
			case <-timer.C:
				{
					bg := time.Now()
					err := func() error {
						var buf []byte
						var err error
						if buf, err = readConfig(); err != nil {
							return errgo.Newf("Error read configuration: %s\n", err)
						}

						if err = parseConfig(buf); err != nil {
							return errgo.Newf("Error parse configuration: %s\n", err)
						}
						return nil
					}()

					if err != nil {
						readerLog.Printf("Service %s - Configuration was read in %6.4f seconds with error. Error: %s\n", confServiceName, time.Since(bg).Seconds(), err)
					} else {
						readerLog.Printf("Service %s - Configuration was read in %6.4f seconds\n", confServiceName, time.Since(bg).Seconds())
					}
					configReadDuration.Set(time.Since(bg).Seconds())
					// Инициируем следующий тик через timeout
					timer.Reset(timeout)

				}
			}
		}
	}(timeout)
	return nil
}

func stopReading() {
	stopChan <- struct{}{}
	<-stoppedChan
}

func readConfig() ([]byte, error) {
	var (
		err error
	)
	if conn != nil {
		if !conn.IsConnected() {
			conn.Free(true)
			conn = nil
		} else {
			if err := conn.Ping(); err != nil {
				conn.Close()
				conn.Free(true)
				conn = nil
			}
		}
	}
	if conn == nil {
		logInfof("Try to login %s@%s\n", reader_username, reader_sid)
		conn, err = oracle.NewConnection(reader_username, reader_password, reader_sid, false)
		if err != nil {
			// Выходим. Прочитать не получиться
			if conn != nil {
				conn.Free(true)
			}
			conn = nil
			return nil, err
		}
	}
	var (
		cur *oracle.Cursor
	)
	cur = conn.NewCursor()
	defer cur.Close()

	if err = cur.Execute(stmReadConfig, []interface{}{configname, hostname}, nil); err != nil {
		return nil, errgo.Newf("error executing `c.config`: %s", otasker.UnMask(err))
	}
	rows, err := cur.FetchAll()
	if err != nil {
		return nil, errgo.Newf("error executing `c.config`: %s", otasker.UnMask(err))
	}
	buf := make([]byte, 0, len(rows)*4000)
	for k := range rows {
		config_line, ok := rows[k][0].(string)
		if !ok {
			return nil, errgo.Newf("data is not String, but %T", rows[k][0])
		}
		buf = append(buf, []byte(config_line)...)
	}
	if bytes.Equal(buf, []byte("{}")) {
		return nil, errgo.Newf("Configuration \"%s\" does not exists", configname)
	}
	return buf, nil
}

const stmReadConfig = `select * from table(c.config(:1, :2, ''))`
