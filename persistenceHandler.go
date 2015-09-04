// persistenceHandler
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/vsdutka/metrics"
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

var numberOfSessions = metrics.NewInt("PersistentHandler_Number_Of_Sessions", "Server - Number of persistent sessions", "Pieces", "p")

type taskInfo struct {
	sessionID         string
	taskID            string
	reqUserName       string
	reqUserPass       string
	reqDumpStatements bool
	reqConnStr        string
	reqParamStoreProc string
	reqBeforeScript   string
	reqAfterScript    string
	reqDocumentTable  string
	reqCGIEnv         map[string]string
	reqProc           string
	reqParams         url.Values
	reqFiles          *otasker.Form
}

type taskTransport struct {
	task       taskInfo
	rcvChannel chan otasker.OracleTaskResult
}

type session struct {
	sync.Mutex
	tasker          otasker.OracleTasker
	sessionID       string
	srcChannel      chan taskTransport
	rcvChannels     map[string]chan otasker.OracleTaskResult
	currTaskID      string
	currTaskStarted time.Time
}

type sessionHandlerUser struct {
	isSpecial bool
	connStr   string
}

var usersFree = sync.Pool{
	New: func() interface{} { return new(sessionHandlerUser) },
}

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
	users              map[string]*sessionHandlerUser
}

type sessionHandler struct {
	srv *applicationServer
	// Конфигурационные параметры
	params           sessionHandlerParams
	paramsMutex      sync.RWMutex
	sessionList      map[string]*session
	sessionListMutex sync.Mutex
	taskerCreator    func() otasker.OracleTasker
}

func newSessionHandler(srv *applicationServer, fn func() otasker.OracleTasker) *sessionHandler {
	h := &sessionHandler{srv: srv,
		params: sessionHandlerParams{
			templates: make(map[string]string),
			users:     make(map[string]*sessionHandlerUser),
		},
		sessionList:   make(map[string]*session),
		taskerCreator: fn,
	}
	return h
}

func (h *sessionHandler) SetConfig(conf *json.RawMessage) {
	type _tUser struct {
		Name      string
		IsSpecial bool
		SID       string
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
		Users              []_tUser     `json:"owa.Users"`
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
			for k, _ := range h.params.users {
				usersFree.Put(h.params.users[k])
				delete(h.params.users, k)
			}
			for k, _ := range t.Users {
				u, ok := usersFree.Get().(*sessionHandlerUser)
				if !ok {
					u = &sessionHandlerUser{
						isSpecial: t.Users[k].IsSpecial,
						connStr:   t.Users[k].SID,
					}
				} else {
					u.isSpecial = t.Users[k].IsSpecial
					u.connStr = t.Users[k].SID
				}
				h.params.users[t.Users[k].Name] = u
			}
		}()
	}
}

func (h *sessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.owaInternalHandler(w, r) {
		return
	}
	r.URL.RawQuery = NSPercentEncoding.FixNonStandardPercentEncoding(r.URL.RawQuery)

	st, ok := h.createTaskInfo(r)
	defer func() {
		for k := range st.reqCGIEnv {
			delete(st.reqCGIEnv, k)
		}
		st = nil
	}()

	if !ok {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"%s%s\"", r.Host, h.RequestUserRealm()))
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized"))
		return
	}

	ses := func() *session {
		h.sessionListMutex.Lock()
		defer h.sessionListMutex.Unlock()

		ses, found := h.sessionList[st.sessionID]
		if !found {
			ses = &session{}
			ses.sessionID = st.sessionID
			ses.srcChannel = make(chan taskTransport, 1000)
			ses.rcvChannels = make(map[string]chan otasker.OracleTaskResult)
			ses.tasker = h.taskerCreator()
			go ses.Listen(h, st.sessionID, h.SessionIdleTimeout())
			h.sessionList[st.sessionID] = ses
			numberOfSessions.Add(1)
		}
		return ses
	}()

	_, p := filepath.Split(path.Clean(r.URL.Path))
	if p == "break_session" {
		//FIXME
		if err := ses.tasker.Break(); err != nil {
			h.responseError(w, err.Error())
		} else {
			h.responseFixedPage(w, "rbreakr", nil)
		}
		return
	}

	res := ses.SendAndRead(st, h.SessionWaitTimeout())

	switch res.StatusCode {
	case otasker.StatusErrorPage:
		{
			h.responseError(w, string(res.Content))
		}
	case otasker.StatusWaitPage:
		{
			s := makeWaitForm(r, st.taskID)

			type DataInfo struct {
				UserName string
				Gmrf     template.HTML
				Duration int64
			}

			h.responseFixedPage(w, "rwait", DataInfo{st.reqUserName, template.HTML(s), res.Duration})
		}
	case otasker.StatusBreakPage:
		{
			s := makeWaitForm(r, st.taskID)

			type DataInfo struct {
				UserName string
				Gmrf     template.HTML
				Duration int64
			}

			h.responseFixedPage(w, "rbreak", DataInfo{st.reqUserName, template.HTML(s), res.Duration})
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

func (h *sessionHandler) removeSessionHandler(sessionID string) {
	h.sessionListMutex.Lock()
	defer h.sessionListMutex.Unlock()
	ses := h.sessionList[sessionID]
	delete(h.sessionList, sessionID)
	ses.Close()
	numberOfSessions.Add(-1)
}

func (h *sessionHandler) createTaskInfo(r *http.Request) (*taskInfo, bool) {
	ok := true
	st := &taskInfo{}
	st.reqUserName, st.reqUserPass, ok = r.BasicAuth()

	remoteUser := st.reqUserName
	if remoteUser == "" {
		remoteUser = "-"
	}

	if !ok {
		if !h.RequestUserInfo() {
			// Авторизация от клиента не требуется.
			// Используем значения по умолчанию
			st.reqUserName = h.DefUserName()
			st.reqUserPass = h.DefUserPass()
		} else {
			return st, false
		}
	} else {
		if !h.RequestUserInfo() {
			// Авторизация от клиента не требуется.
			// Используем значения по умолчанию
			st.reqUserName = h.DefUserName()
			st.reqUserPass = h.DefUserPass()
		}
	}
	st.reqFiles, _ = otasker.ParseMultipartFormEx(r, 64<<20)

	isSpecial, connStr := h.userInfo(st.reqUserName)
	if connStr == "" {
		return st, false
	}
	st.sessionID = makeHandlerID(isSpecial, st.reqUserName, st.reqUserPass, r.Header.Get("DebugIP"), r)
	st.taskID = makeTaskID(r)
	st.reqConnStr = connStr
	st.reqDocumentTable = h.DocumentTable()
	st.reqParamStoreProc = h.ParamStoreProc()
	st.reqBeforeScript = h.BeforeScript()
	st.reqAfterScript = h.AfterScript()
	st.reqCGIEnv = makeEnvParams(r, st.reqDocumentTable, remoteUser, h.RequestUserRealm()+"/")

	st.reqParams = r.Form

	_, st.reqProc = filepath.Split(path.Clean(r.URL.Path))
	return st, true
}

func (ses *session) Listen(h *sessionHandler, sessionID string, idleTimeout time.Duration) {
	defer h.removeSessionHandler(sessionID)
	for {
		select {
		case transport := <-ses.srcChannel:
			{
				res := func() otasker.OracleTaskResult {
					ses.setCurrentTaskID(transport.task.taskID)
					defer func() {
						ses.setCurrentTaskID("")
					}()

					return ses.tasker.Run(transport.task.sessionID,
						transport.task.taskID,
						transport.task.reqUserName,
						transport.task.reqUserPass,
						transport.task.reqConnStr,
						transport.task.reqParamStoreProc,
						transport.task.reqBeforeScript,
						transport.task.reqAfterScript,
						transport.task.reqDocumentTable,
						transport.task.reqCGIEnv,
						transport.task.reqProc,
						transport.task.reqParams,
						transport.task.reqFiles,
						srv.expandFileName(fmt.Sprintf("${log_dir}\\err_%s_${datetime}.log", transport.task.reqUserName)))
				}()
				transport.rcvChannel <- res
				if res.StatusCode == otasker.StatusRequestWasInterrupted {
					return
				}
			}
		case <-time.After(idleTimeout):
			{
				return
			}
		}
	}
}

func (ses *session) getCurrentTaskInfo() (string, time.Time) {
	ses.Lock()
	defer ses.Unlock()
	return ses.currTaskID, ses.currTaskStarted
}

func (ses *session) setCurrentTaskID(taskID string) {
	ses.Lock()
	defer ses.Unlock()
	if taskID == "" {
		ses.currTaskStarted = time.Time{}
	} else {
		ses.currTaskStarted = time.Now()
	}
	ses.currTaskID = taskID
}

func (ses *session) send(task *taskInfo) chan otasker.OracleTaskResult {
	ses.Lock()
	defer ses.Unlock()

	r, ok := ses.rcvChannels[task.taskID]
	if !ok {
		// Канал делаем буферизованным. Если даже никто не ждет получения ответа,
		// все равно произойдет выход из Listen
		r = make(chan otasker.OracleTaskResult, 1)
		t := taskTransport{*task, r}
		ses.rcvChannels[task.taskID] = r
		ses.srcChannel <- t
	}

	return r
}

func (ses *session) SendAndRead(task *taskInfo, timeOut time.Duration) otasker.OracleTaskResult {
	r := ses.send(task)
	for {
		select {
		case res := <-r:
			{
				// Дождались результатов. Отдаем клиенту

				// Удаляем из списка каналов для получения результатов ранее отправленных запросов
				func() {
					ses.Lock()
					defer ses.Unlock()
					delete(ses.rcvChannels, task.taskID)
				}()
				return res
			}
		case <-time.After(timeOut):
			{
				taskID, taskSarted := ses.getCurrentTaskInfo()
				if taskID == task.taskID {
					/* Сигнализируем о том, что идет выполнение этого запроса и нужно показать червяка */
					return otasker.OracleTaskResult{StatusCode: otasker.StatusWaitPage, Duration: int64(time.Since(taskSarted) / time.Second)}
				}
				/* Сигнализируем о том, что идет выполнение этого запроса и нужно показать червяка */
				return otasker.OracleTaskResult{StatusCode: otasker.StatusBreakPage, Duration: int64(time.Since(taskSarted) / time.Second)}
			}
		}
	}
}

func (h *sessionHandler) owaInternalHandler(rw http.ResponseWriter, r *http.Request) bool {
	_, p := filepath.Split(path.Clean(r.URL.Path))
	if p == "!" {
		s := func() struct{ Sessions []otasker.OracleTaskInfo } {
			h.sessionListMutex.Lock()
			defer h.sessionListMutex.Unlock()
			res := struct {
				Sessions []otasker.OracleTaskInfo
			}{make([]otasker.OracleTaskInfo, 0)}

			for _, val := range h.sessionList {
				res.Sessions = append(res.Sessions, val.tasker.Info())
			}
			return res
		}()

		h.responseFixedPage(rw, "sessions", s)

		return true
	}
	return false
}

func (ses *session) Close() {
	ses.Lock()
	defer ses.Unlock()
	close(ses.srcChannel)
	for _, v := range ses.rcvChannels {
		close(v)
	}
	if ses.tasker != nil {
		// Очистку объекта делаем асинхронной, поскольку она ждет закрытия курсоров
		go ses.tasker.CloseAndFree()
		ses.tasker = nil
	}
}
func (h *sessionHandler) SessionIdleTimeout() time.Duration {
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	return time.Duration(h.params.sessionIdleTimeout) * time.Millisecond
}
func (h *sessionHandler) SessionWaitTimeout() time.Duration {
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	return time.Duration(h.params.sessionWaitTimeout) * time.Millisecond
}
func (h *sessionHandler) RequestUserInfo() bool {
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	return h.params.requestUserInfo
}
func (h *sessionHandler) RequestUserRealm() string {
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	return h.params.requestUserRealm
}
func (h *sessionHandler) DefUserName() string {
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	return h.params.defUserName
}
func (h *sessionHandler) DefUserPass() string {
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	return h.params.defUserPass
}
func (h *sessionHandler) BeforeScript() string {
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	return h.params.beforeScript
}
func (h *sessionHandler) AfterScript() string {
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	return h.params.afterScript
}
func (h *sessionHandler) ParamStoreProc() string {
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	return h.params.paramStoreProc
}
func (h *sessionHandler) DocumentTable() string {
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	return h.params.documentTable
}

func (h *sessionHandler) templateBody(templateName string) (string, bool) {
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
		fmt.Println("responseFixedPage: ", err.Error())
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

func (h *sessionHandler) userInfo(user string) (isSpecial bool, connStr string) {
	h.paramsMutex.RLock()
	defer h.paramsMutex.RUnlock()
	u, ok := h.params.users[strings.ToUpper(user)]
	if !ok {
		return false, ""
	}
	return u.isSpecial, u.connStr
}
