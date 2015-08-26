// appServer
package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/kardianos/osext"
	"github.com/vsdutka/metrics"
	"github.com/vsdutka/otasker"
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

type applicationServer struct {
	basePath         string
	serviceName      string
	serviceDispName  string
	httpPort         int
	httpDebugPort    int
	httpReadTimeout  int
	httpWriteTimeout int
	httpSsl          bool
	httpSslCert      string
	httpSslKey       string
	httpLogDir       string
	configMutex      sync.RWMutex
	configReaded     bool
	//	configReadedWg   sync.WaitGroup
	//	configReader     *configReader

	mux *HttpServeMux
}

func newApplicationServer() *applicationServer {
	s := applicationServer{mux: NewHttpServeMux(), configReaded: false}
	exeName, err := osext.Executable()

	if err == nil {
		exeName, err = filepath.Abs(exeName)
		if err == nil {
			s.basePath = filepath.Dir(exeName)
		}
	}
	return &s
}

var connCounter = metrics.NewInt("open_connections", "HTTP - Number of open connections", "", "")

func (s *applicationServer) Start() {
	if s.HTTPDebugPort() != 0 {
		logInfof("Debug listener starting on port \"%d\"\n", s.HTTPDebugPort())
	}
	go func(HttpDebugPort int) {
		if HttpDebugPort != 0 {
			if err := http.ListenAndServe(fmt.Sprintf(":%d", HttpDebugPort), nil); err != nil {
				logError(err)
			}
		}
	}(s.HTTPDebugPort())
	if s.HTTPSsl() {
		logInfof("Main listener starting on port \"%d\" with SSL support\n", s.HTTPPort())
	} else {
		logInfof("Main listener starting on port \"%d\"\n", s.HTTPPort())
	}
	go func() {
		serverHTTP := &http.Server{Addr: fmt.Sprintf(":%d", s.HTTPPort()),
			ReadTimeout:  time.Duration(s.HTTPReadTimeout()) * time.Millisecond,
			WriteTimeout: time.Duration(s.HTTPWriteTimeout()) * time.Millisecond,
			//Позволяет отслеживать состояние клиентского соединения
			ConnState: func(conn net.Conn, cs http.ConnState) {
				switch cs {
				case http.StateNew:
					connCounter.Add(1)
				case http.StateClosed:
					connCounter.Add(-1)
				}
			},
			Handler: WriteLog(s.mux, s)}

		if s.HTTPSsl() {
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
				config.Certificates[0], err = tls.X509KeyPair([]byte(s.HTTPSslCert()), []byte(s.HTTPSslKey()))
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
				logError(err)
			}
		} else {
			if err := serverHTTP.ListenAndServe(); err != nil {
				logError(err)
			}
		}

	}()

}
func (s *applicationServer) Stop() {
	//	s.configReader.shutdown()
}
func (s *applicationServer) ServiceName() string {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return fmt.Sprintf("%s_%d", s.serviceName, s.httpPort)
}
func (s *applicationServer) ServiceDispName() string {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return fmt.Sprintf("%s on %d", s.serviceDispName, s.httpPort)
}

func (s *applicationServer) HTTPPort() int {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return s.httpPort
}
func (s *applicationServer) HTTPDebugPort() int {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return s.httpDebugPort
}
func (s *applicationServer) HTTPReadTimeout() int {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return s.httpReadTimeout
}
func (s *applicationServer) HTTPWriteTimeout() int {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return s.httpWriteTimeout
}
func (s *applicationServer) HTTPSsl() bool {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return s.httpSsl
}
func (s *applicationServer) HTTPSslCert() string {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return s.httpSslCert
}
func (s *applicationServer) HTTPSslKey() string {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return s.httpSslKey
}
func (s *applicationServer) HTTPLogDir() string {
	s.configMutex.RLock()
	defer s.configMutex.RUnlock()
	return s.httpLogDir
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
			return s.expandFileName(s.HTTPLogDir())
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

func (s *applicationServer) setServerConfig(
	serviceName, serviceDispName string,
	httpPort, httpDebugPort, httpReadTimeout, httpWriteTimeout int,
	httpSsl bool, httpSslCert, httpSslKey,
	httpLogDir string,
	handlersConfig []json.RawMessage,
) error {
	if !s.configReaded {
		// Параметры HTTP сервера можно устанавливать только при старте сервера
		func() {
			s.configMutex.Lock()
			defer s.configMutex.Unlock()
			s.serviceName = serviceName
			s.serviceDispName = serviceDispName
			s.httpPort = httpPort
			s.httpDebugPort = httpDebugPort
			s.httpReadTimeout = httpReadTimeout
			s.httpWriteTimeout = httpWriteTimeout
			s.httpSsl = httpSsl
			s.httpSslCert = httpSslCert
			s.httpSslKey = httpSslKey
			s.httpLogDir = httpLogDir
		}()
	}
	type _t struct {
		Path   string `json:"Path"`
		Type   string `json:"Type"`
		Delete bool   `json:"Delete"`
	}
	t := _t{Delete: false}
	for _, v := range handlersConfig {
		//t := _t{Delete: false}
		if err := json.Unmarshal(v, &t); err != nil {
			return err
		} else {
			if t.Delete {
				s.mux.Delete(t.Path + "/")
			} else {
				h, ok := s.mux.GetHandler(t.Path + "/")
				if !ok {
					s.mux.Handle(t.Path+"/", newHandler(s, t.Type, t.Path))
					h, _ = s.mux.GetHandler(t.Path + "/")
				}
				h.SetConfig(&v)
			}
		}
	}
	if !s.configReaded {
		s.configReaded = true
		//		s.configReadedWg.Done()
	}
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
