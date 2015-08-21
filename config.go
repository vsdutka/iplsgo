// config
package main

import (
	"bytes"
	"encoding/json"
	"expvar"
	"github.com/vsdutka/expvarmon"
	"github.com/vsdutka/otasker"
	"gopkg.in/errgo.v1"
	"gopkg.in/goracle.v1/oracle"
	"log"
	"os"
	"time"
)

var (
	configReadDuration = expvar.NewFloat("config_read_duration")
)

func init() {
	expvarmon.RegisterVariableInfo("config_read_duration", "Config read duration", "Seconds", "s")
}

type Config struct {
	stopChan   chan int
	conn       *oracle.Connection
	username   string
	password   string
	sid        string
	configname string
	hostname   string
}

func NewConfig(
	dsn, configName string,
	timeout time.Duration,
	serverCallback func(
		serviceName, serviceDispName string,
		httpPort, httpDebugPort, httpReadTimeout, httpWriteTimeout int,
		httpSsl bool, httpSslCert, httpSslKey,
		httpLogDir string,
		handlersParams []json.RawMessage) error,
) *Config {

	c := &Config{stopChan: make(chan int), configname: configName}

	var (
		err error
		buf []byte
	)

	c.username, c.password, c.sid = oracle.SplitDSN(dsn)
	if c.hostname, err = os.Hostname(); err != nil {
		log.Fatalf("Error getting host name: %s\n", err)
	}

	if buf, err = c.readConfig(); err != nil {
		log.Fatalf("Error read configuration: %s\n", err)
	}

	if err = parseConfig(buf, serverCallback); err != nil {
		log.Fatalf("Error parse configuration: %s\n", err)
	}

	go func() {
		var (
			prevBuf []byte = buf[:len(buf)]
		)
		defer func() {
			if c.conn != nil {
				if c.conn.IsConnected() {
					c.conn.Close()
				}
			}
		}()

		timer := time.NewTimer(timeout)

		for {
			select {
			case <-c.stopChan:
				{
					return
				}
			case <-timer.C:
				{
					timer.Reset(timeout)

					bg := time.Now()
					err = func() (err error) {
						var buf []byte
						if buf, err = c.readConfig(); err != nil {
							return errgo.Newf("Error read configuration: %s\n", err)
						}
						if !bytes.Equal(prevBuf, buf) {
							if err = parseConfig(buf, serverCallback); err != nil {
								return errgo.Newf("Error parse configuration: %s\n", err)
							}
							prevBuf = buf[:len(buf)]
						}
						return nil
					}()

					if err != nil {
						logInfof("Configuration was read in %6.4f seconds with error. Error: %s\n", time.Since(bg).Seconds(), err)
						//confLogger.Printf("Configuration was read in %6.4f seconds with error. Error: %s\n", time.Since(bg).Seconds(), err)
					} else {
						logInfof("Configuration was read in %6.4f seconds\n", time.Since(bg).Seconds())
						//confLogger.Printf("Configuration was read in %6.4f seconds\n", time.Since(bg).Seconds())
					}
					configReadDuration.Set(time.Since(bg).Seconds())
				}
			}
		}
	}()

	return c
}

func (c *Config) Stop() {
	c.stopChan <- 1
}

func (c *Config) readConfig() ([]byte, error) {
	var (
		err error
	)
	if c.conn == nil {
		c.conn, err = oracle.NewConnection(c.username, c.password, c.sid, false)
		if err != nil {
			// Выходим. Прочитать не получиться
			c.conn = nil
			return nil, err
		}
	} else {
		err = c.conn.Ping()
		if err != nil {
			c.conn.Close()
			c.conn, err = oracle.NewConnection(c.username, c.password, c.sid, false)
			if err != nil {
				// Выходим. Прочитать не получиться
				c.conn = nil
				return nil, err
			}
		}
	}

	var (
		cur          *oracle.Cursor
		confNameVar  *oracle.Variable
		hostNameVar  *oracle.Variable
		confLinesVar *oracle.Variable
	)
	cur = c.conn.NewCursor()
	defer cur.Close()
	if confNameVar, err = cur.NewVar(c.configname); err != nil {
		return nil, errgo.Newf("error creating variable for %s(%T): %s", c.configname, c.configname, err)
	}

	if hostNameVar, err = cur.NewVar(c.hostname); err != nil {
		return nil, errgo.Newf("error creating variable for %s(%T): %s", c.hostname, c.hostname, err)
	}

	if confLinesVar, err = cur.NewVariable(0, oracle.ClobVarType, 0); err != nil {
		return nil, errgo.Newf("error creating variable for %s(%T): %s", "ClobVarType", "ClobVarType", err)
	}
	defer confLinesVar.Free()

	if err = cur.Execute(stm_read_config, nil, map[string]interface{}{"ainstance_name": confNameVar, "ahost_name": hostNameVar, "confLines": confLinesVar}); err != nil {
		return nil, errgo.Newf("error executing `c.config`: %s", otasker.UnMask(err))
	}

	data, err := confLinesVar.GetValue(0)
	if err != nil {
		return nil, err
	}
	ext, ok := data.(*oracle.ExternalLobVar)
	if !ok {
		return nil, errgo.Newf("data is not *ExternalLobVar, but %T", data)
	}
	if ext != nil {
		size, err := ext.Size(false)
		if err != nil {
			return nil, errgo.Newf("size error: ", err)
		}
		if size != 0 {
			buf, err := ext.ReadAll()
			if err != nil {
				return nil, err
			}
			return buf, nil
		}
		return nil, errgo.New("data size is 0")
	}
	return nil, errgo.New("data not available")
}

func parseConfig(
	buf []byte,
	serverCallback func(
		serviceName, serviceDispName string,
		httpPort, httpDebugPort, httpReadTimeout, httpWriteTimeout int,
		httpSsl bool, httpSslCert, httpSslKey,
		httpLogDir string,
		handlersParams []json.RawMessage) error,
) error {
	type _t struct {
		ServiceName      string            `json:"Service.Name"`
		ServiceDispName  string            `json:"Service.DisplayName"`
		HTTPPort         int               `json:"Http.Port"`
		HTTPDebugPort    int               `json:"Http.DebugPort"`
		HTTPReadTimeout  int               `json:"Http.ReadTimeout"`
		HTTPWriteTimeout int               `json:"Http.WriteTimeout"`
		HTTPSsl          bool              `json:"Http.SSL"`
		HTTPSslCert      string            `json:"Http.SSLCert"`
		HTTPSslKey       string            `json:"Http.SSLKey"`
		HTTPLogDir       string            `json:"Http.LogDir"`
		List             []json.RawMessage `json:"Http.Handlers"`
	}
	var appServerConfig _t = _t{
		ServiceName:      "iPLSGo",
		ServiceDispName:  "iPLSGo Server",
		HTTPPort:         10111,
		HTTPDebugPort:    0,
		HTTPReadTimeout:  15000,
		HTTPWriteTimeout: 15000,
		HTTPSsl:          false,
		HTTPSslCert:      "",
		HTTPSslKey:       "",
		HTTPLogDir:       "${app_dir}\\log\\"}

	err := json.Unmarshal(buf, &appServerConfig)
	if err != nil {
		return errgo.Newf("error parsing configuration: %s", err)
	}
	return serverCallback(
		appServerConfig.ServiceName,
		appServerConfig.ServiceDispName,
		appServerConfig.HTTPPort,
		appServerConfig.HTTPDebugPort,
		appServerConfig.HTTPReadTimeout,
		appServerConfig.HTTPWriteTimeout,
		appServerConfig.HTTPSsl,
		appServerConfig.HTTPSslCert,
		appServerConfig.HTTPSslKey,
		appServerConfig.HTTPLogDir,
		appServerConfig.List,
	)
}

const stm_read_config = `
begin
  :confLines := c.config(ainstance_name => :ainstance_name,
                      ahost_name => :ahost_name);
  dbms_session.modify_package_state(dbms_session.reinitialize);
end;
`
