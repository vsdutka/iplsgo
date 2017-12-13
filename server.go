// server
package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/kardianos/osext"
	"gopkg.in/errgo.v1"

	"github.com/vsdutka/iplsgo/otasker"
	"github.com/vsdutka/metrics"
	"github.com/vsdutka/mltpart"
	"github.com/vsdutka/nspercent-encoding"
)

const (
	defHTTPtimeout = 2400000
)

var (
	confLock             sync.RWMutex
	confServerReaded     = false
	confServiceName      string
	confServiceDispName  string
	confHTTPPort         int
	confHTTPDebugPort    int
	confHTTPReadTimeout  int
	confHTTPWriteTimeout int
	confHTTPSsl          bool
	confHTTPSslCert      string
	confHTTPSslKey       string
	confHTTPLogDir       string
	basePath             string
	prevConf             []byte

	router *httprouter.Router
)

var connCounter = metrics.NewInt("open_connections", "HTTP - Number of open connections", "", "")

func startServer() {
	go func() {
		if confHTTPDebugPort != 0 {
			logInfof("Debug listener starting on port \"%d\"\n", confHTTPDebugPort)
			debugHTTP := &http.Server{Addr: fmt.Sprintf(":%d", confHTTPDebugPort),
				ReadTimeout:  time.Duration(confHTTPReadTimeout) * time.Millisecond,
				WriteTimeout: time.Duration(confHTTPReadTimeout) * time.Millisecond,
				Handler:      http.DefaultServeMux,
			}
			if err := debugHTTP.ListenAndServe(); err != nil {
				logError(err)
			}
		}
	}()
	if confHTTPSsl {
		logInfof("Main listener starting on port \"%d\" with SSL support\n", confHTTPPort)
	} else {
		logInfof("Main listener starting on port \"%d\"\n", confHTTPPort)
	}
	go func() {
		serverHTTP := &http.Server{Addr: fmt.Sprintf(":%d", confHTTPPort),
			ReadTimeout:  time.Duration(confHTTPReadTimeout) * time.Millisecond,
			WriteTimeout: time.Duration(confHTTPWriteTimeout) * time.Millisecond,
			//Позволяет отслеживать состояние клиентского соединения
			ConnState: func(conn net.Conn, cs http.ConnState) {
				switch cs {
				case http.StateNew:
					connCounter.Add(1)
				case http.StateClosed:
					connCounter.Add(-1)
				}
			},
			Handler: http.DefaultServeMux}

		if confHTTPSsl {
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
				config.Certificates[0], err = tls.X509KeyPair([]byte(confHTTPSslCert), []byte(confHTTPSslKey))
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
func stopServer() {
	//	s.configReader.shutdown()
}

func init() {
	http.HandleFunc("/", serveHTTP)
	exeName, err := osext.Executable()

	if err == nil {
		exeName, err = filepath.Abs(exeName)
		if err == nil {
			basePath = filepath.Dir(exeName)
		}
	}
}

func serveHTTP(w http.ResponseWriter, r *http.Request) {
	rt := func() *httprouter.Router {
		confLock.RLock()
		defer confLock.RUnlock()
		return router
	}()
	r.URL.Path = strings.ToLower(r.URL.Path)

	countOfRequests.Add(1)
	defer countOfRequests.Add(-1)

	start := time.Now()
	writer := statusWriter{w, 0, 0}
	rt.ServeHTTP(&writer, r)
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

	writeToLog(fmt.Sprintf("%s, %s, %s, %s, %s, %s, %d, %d, %d, %d, %s, %s, %v\r\n",
		r.RemoteAddr,
		user,
		end.Format("2006.01.02"),
		end.Format("15:04:05.000000000"),
		r.Proto,
		r.Host,
		length,
		r.ContentLength,
		time.Since(start)/time.Millisecond,
		statusCode,
		r.Method,
		url,
		latency,
	))
}

func resetConfig() {
	confLock.Lock()
	defer confLock.Unlock()
	// -- //
	confServiceName = ""
	confServiceDispName = ""
	confHTTPPort = 0
	confHTTPDebugPort = 0
	confHTTPReadTimeout = defHTTPtimeout
	confHTTPWriteTimeout = defHTTPtimeout
	confHTTPSsl = false
	confHTTPSslCert = ""
	confHTTPSslKey = ""
	confHTTPLogDir = ""
	confServerReaded = false
	// -- //
	updateUsers(nil)
	// -- //
	router = httprouter.New()
	prevConf = []byte{}
}
func parseConfig(buf []byte) error {
	if bytes.Equal(prevConf, buf) {
		return nil
	}

	var c = serverConfigHolder{
		HTTPReadTimeout:  defHTTPtimeout,
		HTTPWriteTimeout: defHTTPtimeout,
		HTTPLogDir:       "${app_dir}\\log\\",
	}

	err := json.Unmarshal(buf, &c)
	if err != nil {
		return errgo.Newf("error parsing configuration: %s", err)
	}

	func() {
		newRouter := httprouter.New()

		for k := range c.Handlers {
			if c.Handlers[k].Path == "" {
				continue
			}

			upath := strings.ToLower(c.Handlers[k].Path)

			switch c.Handlers[k].Type {
			case "Redirect":
				{
					newRouter.GET(upath, newRedirect(c.Handlers[k].RedirectPath))

				}
			case "Static":
				{
					newRouter.ServeFiles(upath+"/*filepath", http.Dir(c.Handlers[k].RootDir))
				}
			case "owa_apex", "owa_classic", "owa_ekb":
				{
					var typeTasker int
					switch c.Handlers[k].Type {
					case "owa_apex":
						typeTasker = otasker.ApexTasker
					case "owa_classic":
						typeTasker = otasker.ClassicTasker
					case "owa_ekb":
						typeTasker = otasker.EkbTasker
					}

					templates := map[string]string{}
					for _, v1 := range c.Handlers[k].Templates {
						templates[v1.Code] = v1.Body
					}
					grps := map[int32]string{}

					for _, v1 := range c.Handlers[k].Grps {
						grps[v1.ID] = v1.SID
					}

					f := newOwa(upath, typeTasker,
						time.Duration(c.Handlers[k].SessionIdleTimeout)*time.Millisecond,
						time.Duration(c.Handlers[k].SessionWaitTimeout)*time.Millisecond,
						c.Handlers[k].RequestUserInfo, c.Handlers[k].RequestUserRealm,
						c.Handlers[k].DefUserName, c.Handlers[k].DefUserPass,
						c.Handlers[k].BeforeScript, c.Handlers[k].AfterScript,
						c.Handlers[k].ParamStoreProc, c.Handlers[k].DocumentTable,
						templates, grps)

					newRouter.GET(upath+"/*proc", f)
					newRouter.POST(upath+"/*proc", f)
				}

			case "SOAP":
				{
					newRouter.GET(upath+"/*proc", newSoap(upath, c.Handlers[k].SoapUserName, c.Handlers[k].SoapUserPass, c.Handlers[k].SoapConnStr))
					newRouter.POST(upath+"/*proc", newSoap(upath, c.Handlers[k].SoapUserName, c.Handlers[k].SoapUserPass, c.Handlers[k].SoapConnStr))
				}

			}
		}
		newRouter.GET("/debug/conf/server", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			c := serverConfigHolder{
				ServiceName:      confServiceName,
				ServiceDispName:  confServiceDispName,
				HTTPPort:         confHTTPPort,
				HTTPDebugPort:    confHTTPDebugPort,
				HTTPReadTimeout:  confHTTPReadTimeout,
				HTTPWriteTimeout: confHTTPWriteTimeout,
				HTTPSsl:          confHTTPSsl,
				HTTPSslCert:      confHTTPSslCert,
				HTTPSslKey:       confHTTPSslKey,
				HTTPLogDir:       confHTTPLogDir,
			}
			buf, err := json.Marshal(c)
			if err != nil {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(err.Error()))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write(buf)
		})
		newRouter.GET("/debug/conf/users", func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
			buf, err := getUsers()
			if err != nil {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(err.Error()))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write(buf)
		})
		// Начинаем изменять параметры
		confLock.Lock()
		defer confLock.Unlock()

		// -- //
		if !confServerReaded {
			confServiceName = fmt.Sprintf("%s_%d", c.ServiceName, c.HTTPPort)
			confServiceDispName = c.ServiceDispName
			confHTTPPort = c.HTTPPort
			confHTTPDebugPort = c.HTTPDebugPort
			confHTTPReadTimeout = c.HTTPReadTimeout
			confHTTPWriteTimeout = c.HTTPWriteTimeout
			confHTTPSsl = c.HTTPSsl
			confHTTPSslCert = c.HTTPSslCert
			confHTTPSslKey = c.HTTPSslKey
			confHTTPLogDir = c.HTTPLogDir
			confServerReaded = true
		}
		// -- //
		updateUsers(c.HTTPUsers)
		// -- //
		router = newRouter
		// -- //
		copy(prevConf, buf)
	}()
	return nil
}

func newRedirect(redirectPath string) func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		http.Redirect(w, r, redirectPath, http.StatusMovedPermanently)
	}
}

func newOwa(pathStr string, typeTasker int, sessionIdleTimeout, sessionWaitTimeout time.Duration, requestUserInfo bool,
	requestUserRealm, defUserName, defUserPass, beforeScript,
	afterScript, paramStoreProc, documentTable string,
	templates map[string]string, grps map[int32]string,
) func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {

	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {

		r.URL.RawQuery = NSPercentEncoding.FixNonStandardPercentEncoding(r.URL.RawQuery)
		//dirName, procName := filepath.Split(path.Clean(r.URL.Path[len(pathStr):]))
		procName := path.Clean(r.URL.Path[len(pathStr)+1:])

		vpath := pathStr

		reqFiles, _ := mltpart.ParseMultipartFormEx(r, 64<<20)

		if procName == "!" {
			sortKeyName := r.FormValue("Sort")
			responseTemplate(w, "sessions", sessions, struct{ Sessions otasker.OracleTaskersStats }{
				otasker.Collect(vpath, sortKeyName, false)})
			return
		}
		// -- //
		userName, userPass, ok := r.BasicAuth()

		remoteUser := userName
		if remoteUser == "" {
			remoteUser = "-"
		}

		if !requestUserInfo {
			// Авторизация от клиента не требуется.
			// Используем значения по умолчанию
			userName = defUserName
			userPass = defUserPass
		} else {
			if !ok {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"%s%s\"", r.Host, requestUserRealm))
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("Unauthorized"))
				return
			}
		}
		dumpFileName := expandFileName(fmt.Sprintf("${log_dir}\\err_%s_${datetime}.log", userName))

		var isSpecial bool
		isSpecial, connStr := func(user string) (bool, string) {
			if user == "" {
				return false, ""
			}
			isSpecial, grpID, ok := getUserInfo(user)
			if !ok {
				return false, ""
			}
			sid := ""
			if sid, ok = grps[grpID]; !ok {
				return false, ""
			}
			return isSpecial, sid
		}(userName)
		if connStr == "" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"%s%s\"", r.Host, requestUserRealm))
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}

		sessionID := makeHandlerID(isSpecial, userName, userPass, r.Header.Get("DebugIP"), r)
		taskID := makeTaskID(r)

		cgiEnv := makeEnvParams(r, documentTable, remoteUser, requestUserRealm+"/")

		procParams := r.Form

		if procName == "break_session" {
			//FIXME
			if err := otasker.Break(vpath, sessionID); err != nil {
				responseError(w, templates["error"], err.Error())
				return
			}
			responseTemplate(w, "rbreakr", templates["rbreakr"], nil)
			return
		}

		if sessionWaitTimeout < 0 {
			sessionWaitTimeout = math.MaxInt64
		}

		if sessionIdleTimeout < 0 {
			sessionIdleTimeout = math.MaxInt64
		}

		res := otasker.Run(vpath, typeTasker, sessionID, taskID, userName, userPass, connStr,
			paramStoreProc, beforeScript, afterScript, documentTable,
			cgiEnv, procName, procParams, reqFiles,
			sessionWaitTimeout, sessionIdleTimeout, dumpFileName)

		switch res.StatusCode {
		case otasker.StatusErrorPage:
			{
				responseError(w, templates["error"], string(res.Content))
			}
		case otasker.StatusWaitPage:
			{
				s := makeWaitForm(r, taskID)

				type DataInfo struct {
					UserName string
					Gmrf     template.HTML
					Duration int64
				}

				responseTemplate(w, "rwait", templates["rwait"], DataInfo{userName, template.HTML(s), res.Duration})
			}
		case otasker.StatusBreakPage:
			{
				s := makeWaitForm(r, taskID)

				type DataInfo struct {
					UserName string
					Gmrf     template.HTML
					Duration int64
				}

				responseTemplate(w, "rbreak", templates["rbreak"], DataInfo{userName, template.HTML(s), res.Duration})
			}
		case otasker.StatusRequestWasInterrupted:
			{
				responseTemplate(w, "rwi", templates["rwi"], nil)
			}
		case otasker.StatusInvalidUsernameOrPassword:
			{
				w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"%s\"", r.Host))
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("Unauthorized"))
			}
		case otasker.StatusInsufficientPrivileges:
			{
				responseTemplate(w, "InsufficientPrivileges", templates["InsufficientPrivileges"], nil)
			}
		case otasker.StatusAccountIsLocked:
			{
				responseTemplate(w, "AccountIsLocked", templates["AccountIsLocked"], nil)
			}
		default:
			{
				location := ""
				for headerName, headerValues := range res.Headers {
					for _, headerValue := range headerValues {
						switch strings.ToLower(headerName) {
						case "status":
							{
								i, err := strconv.Atoi(headerValue)
								if err == nil {
									res.StatusCode = i
								}
							}
						case "location":
							{
								//FIXME - убрать после того, как поймем, почему APEX генерирует неправильную ссылку
								if strings.HasPrefix(headerValue, "/f?p") {
									headerValue = headerValue[1:]
								}
								location = headerValue
							}
						default:
							{
								w.Header().Add(headerName, headerValue)
							}
						}
					}

				}
				if (res.StatusCode == http.StatusMovedPermanently) || (res.StatusCode == http.StatusFound) {
					http.Redirect(w, r, location, res.StatusCode)
				} else {
					w.Header().Set("Content-Type", res.ContentType)
					w.WriteHeader(res.StatusCode)
					w.Write(res.Content)
				}
			}
		}
	}
}

func expandFileName(fileName string) string {
	return os.Expand(fileName, func(key string) string {
		switch strings.ToUpper(key) {
		case "APP_DIR":
			return basePath
		case "LOG_DIR":
			return expandFileName(confHTTPLogDir)
		case "SERVICE_NAME":
			return confServiceName
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

type serverConfigHolder struct {
	ServiceName      string          `json:"Service.Name"`
	ServiceDispName  string          `json:"Service.DisplayName"`
	HTTPPort         int             `json:"Http.Port"`
	HTTPDebugPort    int             `json:"Http.DebugPort"`
	HTTPReadTimeout  int             `json:"Http.ReadTimeout"`
	HTTPWriteTimeout int             `json:"Http.WriteTimeout"`
	HTTPSsl          bool            `json:"Http.SSL"`
	HTTPSslCert      string          `json:"Http.SSLCert"`
	HTTPSslKey       string          `json:"Http.SSLKey"`
	HTTPLogDir       string          `json:"Http.LogDir"`
	HTTPUsers        json.RawMessage `json:"Http.Users"`
	Handlers         []struct {
		Path               string `json:"Path"`
		Type               string `json:"Type"`
		RootDir            string `json:"RootDir"`
		RedirectPath       string `json:"RedirectPath"`
		SessionIdleTimeout int    `json:"owa.SessionIdleTimeout"`
		SessionWaitTimeout int    `json:"owa.SessionWaitTimeout"`
		RequestUserInfo    bool   `json:"owa.ReqUserInfo"`
		RequestUserRealm   string `json:"owa.ReqUserRealm"`
		DefUserName        string `json:"owa.DBUserName"`
		DefUserPass        string `json:"owa.DBUserPass"`
		BeforeScript       string `json:"owa.BeforeScript"`
		AfterScript        string `json:"owa.AfterScript"`
		ParamStoreProc     string `json:"owa.ParamStroreProc"`
		DocumentTable      string `json:"owa.DocumentTable"`
		Templates          []struct {
			Code string
			Body string
		} `json:"owa.Templates"`
		Grps []struct {
			ID  int32
			SID string
		} `json:"owa.UserGroups"`
		SoapUserName string `json:"soap.DBUserName"`
		SoapUserPass string `json:"soap.DBUserPass"`
		SoapConnStr  string `json:"soap.DBConnStr"`
	} `json:"Http.Handlers"`
}

const (
	sessions = `<HTML>
<HEAD>
<TITLE>Список сессий виртуальной директории</TITLE>
<META HTTP-EQUIV="Expires" CONTENT="0"/>
<script src="https://rolf-asw1:63088/i/libraries/apex/minified/desktop_all.min.js?v=4.2.1.00.08" type="text/javascript"></script>
<style>
  table {
    border: 1px solid black; /* Рамка вокруг таблицы */
    border-collapse: collapse; /* Отображать только одинарные линии */
  }
  th {
    text-align: center; /* Выравнивание по левому краю */
    font-weight:bold;
    background: #ccc; /* Цвет фона ячеек */
    padding: 2px; /* Поля вокруг содержимого ячеек */
    border: 1px solid black; /* Граница вокруг ячеек */
  }
  td {
    padding: 2px; /* Поля вокруг содержимого ячеек */
    border: 1px solid black; /* Граница вокруг ячеек */
    font-family: Arial;
    font-size: 10pt;
  }
</style>

<script>
function dp() {
  if(navigator.appName.indexOf("Microsoft") > -1){
    return "block";
  }
  else {
    return "table-row";
  }
}
function chD(r, rNum, n){
  var v = document.all[n];
  var temp1 = rNum;
  if (v != undefined) {
    if (v.length == undefined) {
      v.style.display = v.style.display=='none' ?  dp() : 'none';
      if (v.style.display=='none') temp1 = 1;
    }
    else {
      for (i=0; i<v.length; i++)
      {
        v[i].style.display = v[i].style.display=='none' ?  dp() : 'none';
      }
      if (v[0].style.display=='none') temp1 = 1;
    }
    $("#"+ r + " td.ch").prop("rowspan", temp1);
  }
}
</script>
</HEAD>
<BODY>
  <H3>Список сессий виртуальной директории</H3>
  <TABLE>
    <thead>
      <TR>
        <th>#</th>
        <th><a href="!?Sort=Created">Создано</a></th>
        <th><a href="!?Sort=UserName">Пользователь</a></th>
        <th><a href="!?Sort=SessionID">Session</a></th>
        <th><a href="!?Sort=Database">Строка соединения</a></th>
        <th><a href="!?Sort=MessageID">Id</a></th>
        <th><a href="!?Sort=NowInProcess">Состояние выполнения</a></th>
		<th><a href="!?Sort=NowInProcess">Шаг</a></th>
        <th><a href="!?Sort=IdleTime">Время простоя, msec</a></th>
        <th><a href="!?Sort=LastDuration">Время выполнения запроса, msec</a></th>
        <th><a href="!?Sort=RequestProceeded">Кол-во запусков на исполнение</a></th>
		<th><a href="!?Sort=ErrorsNumber">Кол-во ошибок</a></th>
      </TR>
    </thead>
{{range $key, $data := .Sessions}}
<TR name="rid{{$key}}" id="rid{{$key}}" STYLE="background-color: {{if eq $data.NowInProcess true}}#00FF00{{else}}white{{end}}; color: black; cursor: Hand;" onClick="{chD('rid{{$key}}',{{$data.StepNum}},'id{{$key}}')}" >
  <TD align="center" class="ch">{{$key}}</TD>
  <TD align="center" nowrap class="ch">{{ $data.Created}}</TD>
  <TD align="center" class="ch">{{ $data.UserName}}</TD>
  <TD align="center" nowrap>{{ $data.SessionID}}</TD>
  <td align="center" nowrap>{{ $data.Database}}</td>
  <TD align="center" nowrap>{{ $data.MessageID}}</TD>
  <TD align="center">{{if eq $data.NowInProcess true}}Выполняется{{else}}Простаивает{{end}}</TD>
  <TD align="center" nowrap>{{ $data.StepName}}</TD>
  <TD align="right">{{ $data.IdleTime}}</TD>
  <TD align="right">{{ $data.LastDuration}}</TD>
  <TD align="right">{{ $data.RequestProceeded}}</TD>
  <TD align="right">{{ $data.ErrorsNumber}}</TD> 
</TR>
{{range $k, $v := $data.LastSteps}}
<tr name="id{{$key}}" id="id{{$key}}" style="display: none; background-color:{{if eq $data.NowInProcess true}}#00FF00{{else}}white{{end}} color: black; cursor: Hand;">
<td>{{$k}}</td>
<td nowrap>{{$v.Name}}</td>
<td align="right">{{$v.Duration}} msec</td>
<td colspan="6"><pre><code class="sql">{{$v.Statement}}</code></pre></td>
</tr>
{{end}}
{{end}}
</TABLE>
</BODY>
</HTML>
`
)
