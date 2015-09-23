// persistenceHandler
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/vsdutka/nspercent-encoding"
	"github.com/vsdutka/otasker"
	"html/template"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type sessionHandlerParams struct {
	sessionIdleTimeout int
	sessionWaitTimeout int
	requestUserInfo    bool
	requestUserRealm   string
	defUserName        string
	defUserPass        string
	beforeScript       string
	afterScript        string
	paramStoreProc     string
	documentTable      string
	templates          map[string]string
	grps               map[int32]string
	typeTasker         int
}

type sessionHandler struct {
	srv *applicationServer
	// Конфигурационные параметры
	params      sessionHandlerParams
	paramsMutex sync.RWMutex
}

func newSessionHandler(srv *applicationServer, typeTasker int) *sessionHandler {
	h := &sessionHandler{srv: srv,
		params: sessionHandlerParams{
			templates:  make(map[string]string),
			grps:       make(map[int32]string),
			typeTasker: typeTasker,
		},
	}
	return h
}

func (h *sessionHandler) SetConfig(conf *json.RawMessage) {
	type _tGrp struct {
		ID  int32
		SID string
	}
	type _tTemplate struct {
		Code string
		Body string
	}
	type _t struct {
		SessionIdleTimeout int          `json:"owa.SessionIdleTimeout"`
		SessionWaitTimeout int          `json:"owa.SessionWaitTimeout"`
		RequestUserInfo    bool         `json:"owa.ReqUserInfo"`
		RequestUserRealm   string       `json:"owa.ReqUserRealm"`
		DefUserName        string       `json:"owa.DBUserName"`
		DefUserPass        string       `json:"owa.DBUserPass"`
		BeforeScript       string       `json:"owa.BeforeScript"`
		AfterScript        string       `json:"owa.AfterScript"`
		ParamStoreProc     string       `json:"owa.ParamStroreProc"`
		DocumentTable      string       `json:"owa.DocumentTable"`
		Templates          []_tTemplate `json:"owa.Templates"`
		//		Users              []_tUser     `json:"owa.Users"`
		Grps []_tGrp `json:"owa.UserGroups"`
	}
	t := _t{}
	if err := json.Unmarshal(*conf, &t); err != nil {
		logError(err)
	} else {
		func() {
			h.paramsMutex.Lock()
			defer func() {
				h.paramsMutex.Unlock()
			}()
			h.params.sessionIdleTimeout = t.SessionIdleTimeout
			h.params.sessionWaitTimeout = t.SessionWaitTimeout
			h.params.requestUserInfo = t.RequestUserInfo
			h.params.requestUserRealm = t.RequestUserRealm
			h.params.defUserName = t.DefUserName
			h.params.defUserPass = t.DefUserPass
			h.params.beforeScript = t.BeforeScript
			h.params.afterScript = t.AfterScript
			h.params.paramStoreProc = t.ParamStoreProc
			h.params.documentTable = t.DocumentTable

			for k, _ := range h.params.templates {
				delete(h.params.templates, k)
			}
			for k, _ := range t.Templates {
				h.params.templates[t.Templates[k].Code] = t.Templates[k].Body
			}

			h.params.grps = make(map[int32]string)
			for k, _ := range t.Grps {
				h.params.grps[t.Grps[k].ID] = t.Grps[k].SID
			}
		}()
	}
}

func (h *sessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.owaInternalHandler(w, r) {
		return
	}
	r.URL.RawQuery = NSPercentEncoding.FixNonStandardPercentEncoding(r.URL.RawQuery)

	vpath, typeTasker, sessionID, taskID, userName, userPass, connStr,
		paramStoreProc, beforeScript, afterScript, documentTable,
		cgiEnv, procName, procParams, reqFiles,
		waitTimeout, idleTimeout, dumpFileName, ok := h.createTaskInfo(r)

	if !ok {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"%s%s\"", r.Host, h.RequestUserRealm()))
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
		return
	}

	if procName == "break_session" {
		//FIXME
		if err := otasker.Break(vpath, sessionID); err != nil {
			h.responseError(w, err.Error())
		} else {
			h.responseFixedPage(w, "rbreakr", nil)
		}
		return
	}

	res := otasker.Run(vpath, typeTasker, sessionID, taskID, userName, userPass, connStr,
		paramStoreProc, beforeScript, afterScript, documentTable,
		cgiEnv, procName, procParams, reqFiles,
		waitTimeout, idleTimeout, dumpFileName)

	switch res.StatusCode {
	case otasker.StatusErrorPage:
		{
			h.responseError(w, string(res.Content))
		}
	case otasker.StatusWaitPage:
		{
			s := makeWaitForm(r, taskID)

			type DataInfo struct {
				UserName string
				Gmrf     template.HTML
				Duration int64
			}

			h.responseFixedPage(w, "rwait", DataInfo{userName, template.HTML(s), res.Duration})
		}
	case otasker.StatusBreakPage:
		{
			s := makeWaitForm(r, taskID)

			type DataInfo struct {
				UserName string
				Gmrf     template.HTML
				Duration int64
			}

			h.responseFixedPage(w, "rbreak", DataInfo{userName, template.HTML(s), res.Duration})
		}
	case otasker.StatusRequestWasInterrupted:
		{
			h.responseFixedPage(w, "rwi", nil)
		}
	case otasker.StatusInvalidUsernameOrPassword:
		{
			w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"%s\"", r.Host))
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
		}
	case otasker.StatusInsufficientPrivileges:
		{
			h.responseFixedPage(w, "InsufficientPrivileges", nil)
		}
	case otasker.StatusAccountIsLocked:
		{
			h.responseFixedPage(w, "AccountIsLocked", nil)
		}
	default:
		{
			if res.Headers != "" {
				for _, s := range strings.Split(res.Headers, "\n") {
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
						switch headerName {
						case "Content-Type":
							{
								res.ContentType = headerValue
							}
						case "Status":
							{
								i, err := strconv.Atoi(headerValue)
								if err == nil {
									res.StatusCode = i
								}
							}
						default:
							{
								w.Header().Set(headerName, headerValue)
							}
						}
					}
				}
			}
			if res.ContentType != "" {
				if mt, _, err := mime.ParseMediaType(res.ContentType); err == nil {
					// Поскольку буфер ВСЕГДА формируем в UTF-8,
					// нужно изменить значение Charset в ContentType
					res.ContentType = mt + "; charset=utf-8"

				}
				w.Header().Set("Content-Type", res.ContentType)
			}
			w.WriteHeader(res.StatusCode)
			w.Write(res.Content)
		}
	}
}

func (h *sessionHandler) createTaskInfo(r *http.Request) (
	vpath string,
	typeTasker int,
	sessionID,
	taskID,
	userName,
	userPass,
	connStr,
	paramStoreProc,
	beforeScript,
	afterScript,
	documentTable string,
	cgiEnv map[string]string,
	procName string,
	procParams url.Values,
	reqFiles *otasker.Form,

	waitTimeout,
	idleTimeout time.Duration,
	dumpFileName string,
	ok bool,
) {
	ok = true
	//Фиксированные параметры
	h.paramsMutex.RLock()

	typeTasker = h.params.typeTasker
	documentTable = h.params.documentTable
	paramStoreProc = h.params.paramStoreProc
	beforeScript = h.params.beforeScript
	afterScript = h.params.afterScript
	requestUserInfo := h.params.requestUserInfo
	defUserName := h.params.defUserName
	defUserPass := h.params.defUserPass
	requestUserRealm := h.params.requestUserRealm
	waitTimeout = time.Duration(h.params.sessionWaitTimeout) * time.Millisecond
	idleTimeout = time.Duration(h.params.sessionIdleTimeout) * time.Millisecond

	h.paramsMutex.RUnlock()

	vpath, procName = filepath.Split(path.Clean(r.URL.Path))

	userName, userPass, ok = r.BasicAuth()

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
			return vpath, typeTasker, sessionID, taskID, userName, userPass, connStr,
				paramStoreProc, beforeScript, afterScript, documentTable, cgiEnv,
				procName, procParams, reqFiles,
				waitTimeout, idleTimeout, dumpFileName,
				false
		}
	}
	dumpFileName = srv.expandFileName(fmt.Sprintf("${log_dir}\\err_%s_${datetime}.log", userName))

	reqFiles, _ = otasker.ParseMultipartFormEx(r, 64<<20)

	var isSpecial bool
	isSpecial, connStr = h.userInfo(userName)
	if connStr == "" {
		return vpath, typeTasker, sessionID, taskID, userName, userPass, connStr,
			paramStoreProc, beforeScript, afterScript, documentTable, cgiEnv,
			procName, procParams, reqFiles,
			waitTimeout, idleTimeout, dumpFileName,
			false
	}

	sessionID = makeHandlerID(isSpecial, userName, userPass, r.Header.Get("DebugIP"), r)
	taskID = makeTaskID(r)

	cgiEnv = makeEnvParams(r, documentTable, remoteUser, requestUserRealm+"/")

	procParams = r.Form

	return vpath, typeTasker, sessionID, taskID, userName, userPass, connStr,
		paramStoreProc, beforeScript, afterScript, documentTable, cgiEnv,
		procName, procParams, reqFiles,
		waitTimeout, idleTimeout, dumpFileName,
		true
}

func (h *sessionHandler) owaInternalHandler(rw http.ResponseWriter, r *http.Request) bool {
	vpath, p := filepath.Split(path.Clean(r.URL.Path))
	if p == "!" {
		sortKeyName := r.FormValue("Sort")
		h.responseFixedPage(rw, "sessions", struct {
			Sessions otasker.OracleTaskersStats
		}{otasker.Collect(vpath, sortKeyName, false)})

		return true
	}
	return false
}

func (h *sessionHandler) RequestUserRealm() string {
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	return h.params.requestUserRealm
}

func (h *sessionHandler) templateBody(templateName string) (string, bool) {
	if templateName == "sessions" {
		return sessions, true
	}
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	templateBody, ok := h.params.templates[templateName]
	return templateBody, ok
}

func (h *sessionHandler) responseError(res http.ResponseWriter, e string) {
	templateBody, ok := h.templateBody("error")
	if !ok {
		res.Header().Set("Content-Type", "text/plain; charset=utf-8")
		res.WriteHeader(200)
		fmt.Fprintf(res, "Unable to find template for page \"%s\"", "error")
		return
	}

	templ, err := template.New("error").Parse(templateBody)
	if err != nil {
		res.Header().Set("Content-Type", "text/plain; charset=utf-8")
		res.WriteHeader(200)
		fmt.Fprint(res, "Error:", err)
		return
	}

	type ErrorInfo struct{ ErrMsg string }

	res.Header().Set("Content-Type", "text/html; charset=utf-8")
	err = templ.ExecuteTemplate(res, "error", ErrorInfo{e})

	if err != nil {
		res.Header().Set("Content-Type", "text/plain; charset=utf-8")
		res.WriteHeader(200)
		fmt.Fprint(res, "Error:", err)
		return
	}
}

func (h *sessionHandler) responseFixedPage(res http.ResponseWriter, pageName string, data interface{}) {
	templateBody, ok := h.templateBody(pageName)
	if !ok {
		h.responseError(res, fmt.Sprintf("Unable to find template for page \"%s\"", pageName))
		return
	}
	templ, err := template.New(pageName).Parse(templateBody)
	if err != nil {
		h.responseError(res, err.Error())
		return
	}
	var buf bytes.Buffer
	err = templ.ExecuteTemplate(&buf, pageName, data)
	if err != nil {
		h.responseError(res, err.Error())
		return
	}

	res.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := res.Write(buf.Bytes()); err != nil {
		// Тут уже нельзя толкать в сокет, поскольку произошла ошибка при отсулке.
		// Поэтому просто показываем ошибку в логе сервера
		logError("responseFixedPage: ", err.Error())
		return
	}

	//	res.Header().Set("Content-Type", "text/html; charset=utf-8")
	//	err = templ.ExecuteTemplate(res, pageName, data)
	//	if err != nil {
	//		fmt.Println(err.Error())
	//		h.responseError(res, err.Error())
	//		return
	//	}
}

func (h *sessionHandler) userInfo(user string) (bool, string) {
	if user == "" {
		return false, ""
	}
	isSpecial, grpId, ok := GetUserInfo(user)
	if !ok {
		return false, ""
	}
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	//	u, ok := h.params.users[strings.ToUpper(user)]

	if sid, ok := h.params.grps[grpId]; !ok {
		return false, ""
	} else {
		return isSpecial, sid
	}
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

//{{end}}
