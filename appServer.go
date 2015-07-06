// appServer
package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	//_ "expvar"
	"fmt"
	"github.com/kardianos/osext"
	"github.com/vsdutka/otasker"
	"gopkg.in/errgo.v1"
	"gopkg.in/goracle.v1/oracle"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type handlersRawConfig struct {
	List []*json.RawMessage `json:"Http.Handlers"`
}

type applicationServerConfig struct {
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
}
type applicationServer struct {
	basePath        string
	config          applicationServerConfig
	configReaded    bool
	closeConfigChan chan bool
	mux             *HttpServeMux
}

func newApplicationServer() *applicationServer {
	s := applicationServer{mux: NewHttpServeMux(), closeConfigChan: make(chan bool), configReaded: false}
	return &s
}

func (s *applicationServer) Load() {
	exeName, err := osext.Executable()

	if err == nil {
		exeName, err = filepath.Abs(exeName)
		if err == nil {
			s.basePath = filepath.Dir(exeName)
		}
	}
	s.prepareConfigReader(s.closeConfigChan, 10*time.Second)
}

func (s *applicationServer) Start() {
	go func(HttpDebugPort int) {
		if HttpDebugPort != 0 {
			if err := http.ListenAndServe(fmt.Sprintf(":%d", HttpDebugPort), nil); err != nil {
				log.Fatal(err)
			}
		}
	}(s.config.HTTPDebugPort)
	go func(HttpPort int, HttpSsl bool, HttpSslCert string, HttpSslKey string, h http.Handler) {

		serverHTTP := &http.Server{Addr: fmt.Sprintf(":%d", HttpPort),
			ReadTimeout:  time.Duration(s.config.HTTPReadTimeout) * time.Millisecond,
			WriteTimeout: time.Duration(s.config.HTTPWriteTimeout) * time.Millisecond,
			//Позволяет отслеживать состояние клиентского соединения
			//			ConnState: func(conn net.Conn, cs http.ConnState) {
			//				fmt.Println(conn.RemoteAddr(), " - state ", cs)
			//			},
			Handler: WriteLog(h, s)}

		if HttpSsl {
			err := func() error {
				addr := serverHTTP.Addr
				if addr == "" {
					addr = ":https"
				}
				config := &tls.Config{}
				if serverHTTP.TLSConfig != nil {
					*config = *serverHTTP.TLSConfig
				}
				if config.NextProtos == nil {
					config.NextProtos = []string{"HTTP/1.1"}
				}

				var err error
				config.Certificates = make([]tls.Certificate, 1)
				config.Certificates[0], err = tls.X509KeyPair([]byte(HttpSslCert), []byte(HttpSslKey))
				if err != nil {
					return err
				}

				ln, err := net.Listen("tcp", addr)
				if err != nil {
					return err
				}

				tlsListener := tls.NewListener(ln, config)
				return serverHTTP.Serve(tlsListener)
			}()

			if err != nil {
				log.Fatal(err)
			}
			//			if err := serverHttp.ListenAndServeTLS(HttpSslCert, HttpSslKey); err != nil {
			//				log.Fatal(err)
			//			}
		} else {
			if err := serverHTTP.ListenAndServe(); err != nil {
				log.Fatal(err)
			}
		}

	}(s.config.HTTPPort, s.config.HTTPSsl, s.config.HTTPSslCert, s.config.HTTPSslKey, s.mux /*http.DefaultServeMux*/)

}
func (s *applicationServer) Stop() {
	s.closeConfigChan <- true
}
func (s *applicationServer) ServiceName() string {
	return fmt.Sprintf("%s_%d", s.config.ServiceName, s.config.HTTPPort)
}
func (s *applicationServer) ServiceDispName() string {
	return fmt.Sprintf("%s on %d", s.config.ServiceDispName, s.config.HTTPPort)
}

func (s *applicationServer) checkDirExists(fileName string) {
	dir, _ := filepath.Split(fileName)
	os.MkdirAll(dir, os.ModeDir)
}

func (s *applicationServer) expandFileName(fileName string) string {
	return os.Expand(fileName, func(key string) string {
		switch strings.ToUpper(key) {
		case "APP_DIR":
			return s.basePath
		case "LOG_DIR":
			return s.expandFileName(s.config.HTTPLogDir)
		case "SERVICE_NAME":
			return s.ServiceName()
		case "DATE":
			return time.Now().Format("2006_01_02")
		case "TIME":
			return strings.Replace(time.Now().Format("T15_04_05.000000000"), ".", "_", -1)
		case "DATETIME":
			return strings.Replace(time.Now().Format("2006_01_02 15_04_05.000000000"), ".", "_", -1)
		default:
			return ""
		}
	})
}

func (s *applicationServer) setServerConfig(c *applicationServerConfig, l []*json.RawMessage) {
	if !s.configReaded {
		s.config = *c
		s.configReaded = true
	}
	type _t struct {
		Path   string `json:"Path"`
		Type   string `json:"Type"`
		Delete bool   `json:"Delete"`
	}
	t := _t{Delete: false}
	for _, v := range l {
		if err := json.Unmarshal(*v, &t); err != nil {
			fmt.Println(err)
		} else {
			if t.Delete {
				s.mux.Delete(t.Path + "/")
			} else {
				h, ok := s.mux.GetHandler(t.Path + "/")
				if !ok {
					s.mux.Handle(t.Path+"/", newHandler(s, t.Type, t.Path))
					h, _ = s.mux.GetHandler(t.Path + "/")
				}
				h.SetConfig(v)
			}
		}
	}
}

func (s *applicationServer) prepareConfigReader(cancelChan chan bool, timeout time.Duration) {
	var wg sync.WaitGroup
	wg.Add(1)

	go func(cancelChan chan bool, timeout time.Duration) {
		username, password, sid := oracle.SplitDSN(*dsnFlag)
		var conn *oracle.Connection
		defer func() {
			if conn != nil {
				if conn.IsConnected() {
					conn.Close()
				}
			}
		}()

		// Первоначальное чтение конфигурации
		err := s.readConfig(&conn, username, password, sid)
		if err != nil {
			log.Fatalf("Unable to read configuration: %s\n", err)
		}
		wg.Done()

		fileConfLogName := s.expandFileName("${log_dir}\\confReader.log")
		s.checkDirExists(fileConfLogName)
		fileConfLog, err := os.OpenFile(fileConfLogName, os.O_RDWR|os.O_APPEND, 0666)
		if err != nil {
			fileConfLog, err = os.OpenFile(fileConfLogName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				log.Fatalf("Unable to open log file: %s\n", err)
			}
		}
		defer fileConfLog.Close()

		confLogger := log.New(fileConfLog, "", log.Ldate|log.Ltime|log.Lmicroseconds)

		for {
			select {
			case <-cancelChan:
				{
					return
				}
			case <-time.After(timeout):
				{
					bg := time.Now()
					err := s.readConfig(&conn, username, password, sid)
					if err != nil {
						confLogger.Printf("Configuration was read in %6.4f seconds with error. Error: %S\n", time.Since(bg).Seconds(), otasker.UnMask(err))
					} else {
						confLogger.Printf("Configuration was read in %6.4f seconds\n", time.Since(bg).Seconds())
					}
				}

			}
		}
	}(cancelChan, timeout)
	wg.Wait()
}

func (s *applicationServer) readConfig(conn **oracle.Connection, username, password, sid string) error {
	const stm = `declare
  param_name    sys.owa.vc_arr;
  param_val     sys.owa.vc_arr;
  thePage       sys.htp.htbuf_arr;
  thePageLinesQ integer := 10000;
  start_line    integer := 0;
  confLines     sys.htp.htbuf_arr;
begin
  sys.OWA.init_cgi_env(0, param_name, param_val);
  sys.htp.init;
  c.config(ainstance_name => :ainstance_name, ahost_name => :ahost_name);
  sys.htp.flush;
  sys.OWA.GET_PAGE(thePage, thePageLinesQ);
  /* Пропускаем HTTP заголовок */
  for i in 1..thePageLinesQ
  loop
    if thePage(i) = sys.owa.NL_CHAR then
      start_line := i + 1;
      exit;
    end if;
  end loop;
  /* Формируем результирующий буфер */
  for i in start_line..thePageLinesQ
  loop
    :confLines(i - start_line + 1) := thePage(i);
  end loop;
  dbms_session.modify_package_state(dbms_session.reinitialize);
end;`

	var err error

	if *conn == nil {
		*conn, err = oracle.NewConnection(username, password, sid, false)
		if err != nil {
			// Выходим. Прочитать не получиться
			return errgo.Newf("Unable to read configuration: %s", otasker.UnMask(err))
		}
	} else {
		err = (*conn).Ping()
		if err != nil {
			(*conn).Close()
			(*conn), err = oracle.NewConnection(username, password, sid, false)
			if err != nil {
				// Выходим. Прочитать не получиться
				return errgo.Newf("Unable to read configuration: %s", otasker.UnMask(err))
			}
		}
	}
	cur := (*conn).NewCursor()
	defer cur.Close()

	var (
		confNameVar  *oracle.Variable
		hostNameVar  *oracle.Variable
		confLinesVar *oracle.Variable
		hostName     string
	)

	if confNameVar, err = cur.NewVar(*confNameFlag); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", *confNameFlag, *confNameFlag, err)
	}

	if hostName, err = os.Hostname(); err != nil {
		return errgo.Newf("error getting host name: %s", err)
	}

	if hostNameVar, err = cur.NewVar(hostName); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", hostName, hostName, err)
	}

	if confLinesVar, err = cur.NewArrayVar(oracle.StringVarType, make([]interface{}, 10000), 256); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", confLinesVar, confLinesVar, err)
	}

	if err = cur.Execute(stm, nil, map[string]interface{}{"ainstance_name": confNameVar, "ahost_name": hostNameVar, "confLines": confLinesVar}); err != nil {
		return errgo.Newf("error executing `c.config`: %s", otasker.UnMask(err))
	}

	var buffer bytes.Buffer
	defer buffer.Reset()

	for i := 0; i < int(confLinesVar.ArrayLength()); i++ {
		s, err := confLinesVar.GetValue(uint(i))
		if err != nil {
			return errgo.Newf("cannot get out value for lines: %s", err)
		}
		//buf = buf + s.(string)
		buffer.WriteString(s.(string))
	}

	appServerConfig := applicationServerConfig{
		ServiceName:      "iPLSGo",
		ServiceDispName:  "iPLSGo Server",
		HTTPPort:         10111,
		HTTPReadTimeout:  15000,
		HTTPWriteTimeout: 15000,
		HTTPSsl:          false,
		HTTPSslCert:      "",
		HTTPSslKey:       "",
		HTTPLogDir:       ".\\log\\"}

	hsRawConfig := handlersRawConfig{}

	dec := json.NewDecoder(strings.NewReader(buffer.String()))

	err = dec.Decode(&struct {
		*applicationServerConfig
		*handlersRawConfig
		//*HandlersConfig
	}{&appServerConfig, &hsRawConfig})

	if err != nil {
		return errgo.Newf("error parsing configuration: %s", err)
	}

	s.setServerConfig(&appServerConfig, hsRawConfig.List)
	return nil
}

func (s *applicationServer) writeTraceFile(fileName, traceInfo string) {
	name := s.expandFileName(fileName)
	s.checkDirExists(name)
	ioutil.WriteFile(name, []byte(traceInfo), 0644)
}

func newHandler(srv *applicationServer, hType, hPath string) HttpHandler {
	switch hType {
	case "Redirect":
		{
			return RedirectHandler("", http.StatusMovedPermanently)
		}
	case "Static":
		{
			return HttpFileServer(hPath, "")
		}
	case "owa_apex":
		{
			return newSessionHandler(srv, otasker.NewOwaApexProcRunner())
		}
	case "owa_classic":
		{
			return newSessionHandler(srv, otasker.NewOwaClassicProcRunner())
		}
	case "owa_ekb":
		{
			return newSessionHandler(srv, otasker.NewOwaEkbProcRunner())

		}
	}
	log.Fatalf("Invalid handler type '%s'", hType)
	return nil
}
