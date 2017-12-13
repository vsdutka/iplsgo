// taskerOra
package otasker

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/vsdutka/mltpart"
	"gopkg.in/errgo.v1"
	"gopkg.in/goracle.v1/oracle"
)

const (
	StatusErrorPage                 = 561
	StatusWaitPage                  = 562
	StatusBreakPage                 = 563
	StatusRequestWasInterrupted     = 564
	StatusInvalidUsernameOrPassword = 565
	StatusInsufficientPrivileges    = 566
	StatusAccountIsLocked           = 567
)

type OracleTaskResult struct {
	StatusCode  int
	ContentType string
	Headers     http.Header
	Content     []byte
	Duration    int64
}
type OracleTasker interface {
	Run(sessionID,
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
		urlParams url.Values,
		reqFiles *mltpart.Form,
		dumpErrorFileName string) OracleTaskResult
	CloseAndFree() error
	Break() error
}

type oracleTaskerStep struct {
	stepID             int
	stepName           string
	stepBg             time.Time
	stepFn             time.Time
	stepStm            string
	stepStmForShowning string
	stepSuccess        bool
}

const (
	stepConnectNum = iota
	stepEvalSid
	stepDescribeNum
	stepSaveFileToDBNum
	stepRunNum
	stepChunkGetNum
	stepDisconnectNum
)

type oracleTasker struct {
	mt                  sync.Mutex //включается только при изменении данных, используемых в Break() и Info()
	cafMutex            sync.Mutex //требуется для синхронизации разрушения объекта C(lose )A(nd )F(ree )Mutex
	opLoggerName        string
	streamID            string
	conn                *oracle.Connection
	connUserName        string
	connUserPass        string
	connStr             string
	sessID              string
	logRequestProceeded int
	logErrorsNum        int
	logSessionID        string
	logTaskID           string
	logUserName         string
	logUserPass         string
	logConnStr          string
	logProcName         string
	logSteps            map[int]*oracleTaskerStep

	stateIsWorking    bool
	stateCreateDT     time.Time
	stateLastFinishDT time.Time
	stmEvalSessionID  string
	stmMain           string
	stmGetRestChunk   string
	stmKillSession    string
	stmFileUpload     string
}

var stepsFree = sync.Pool{
	New: func() interface{} { return new(oracleTaskerStep) },
}

func newTasker(stmEvalSessionID, stmMain, stmGetRestChunk, stmKillSession, stmFileUpload string) oracleTasker {
	return oracleTasker{
		stateIsWorking:    false,
		stateCreateDT:     time.Now(),
		stateLastFinishDT: time.Time{},
		logSteps:          make(map[int]*oracleTaskerStep),

		stmEvalSessionID: stmEvalSessionID,
		stmMain:          stmMain,
		stmGetRestChunk:  stmGetRestChunk,
		stmKillSession:   stmKillSession,
		stmFileUpload:    stmFileUpload,
	}
}

func newTaskerIntf(stmEvalSessionID, stmMain, stmGetRestChunk, stmKillSession, stmFileUpload string) OracleTasker {
	r := newTasker(stmEvalSessionID, stmMain, stmGetRestChunk, stmKillSession, stmFileUpload)
	return &r
}

func (r *oracleTasker) initLog() {
	if r.logSteps != nil {
		for k := range r.logSteps {
			s := r.logSteps[k]
			s.stepName = ""
			s.stepStm = ""
			s.stepStmForShowning = ""
			stepsFree.Put(s)
			delete(r.logSteps, k)
		}
	}
	r.logSessionID = ""
	r.logTaskID = ""
	r.logUserName = ""
	r.logUserPass = ""
	r.logConnStr = ""
	r.logProcName = ""
	r.logSteps = make(map[int]*oracleTaskerStep)
}

func (r *oracleTasker) CloseAndFree() error {
	r.cafMutex.Lock()
	defer r.cafMutex.Unlock()

	if r.conn != nil {
		if err := r.conn.Close(); err != nil {
			return err
		}
	}
	r.initLog()

	return nil
}

func (r *oracleTasker) Run(sessionID, taskID, userName, userPass, connStr,
	paramStoreProc, beforeScript, afterScript, documentTable string,
	cgiEnv map[string]string, procName string, urlParams url.Values,
	reqFiles *mltpart.Form, dumpErrorFileName string) OracleTaskResult {

	r.cafMutex.Lock()

	func() {
		r.mt.Lock()
		defer r.mt.Unlock()
		r.initLog()
		r.logRequestProceeded++
		r.logSessionID = sessionID
		r.logTaskID = taskID
		r.logUserName = userName
		r.logUserPass = userPass
		r.logConnStr = connStr
		r.logProcName = procName
		r.stateIsWorking = true
	}()

	defer func() {

		func() {
			r.mt.Lock()
			defer r.mt.Unlock()
			r.stateIsWorking = false
			r.stateLastFinishDT = time.Now()
		}()

		r.cafMutex.Unlock()
	}()

	if len(cgiEnv) == 0 {
		fmt.Println("Run len(cgiEnv) == 0")
		os.Exit(-1)
	}

	bg := time.Now()
	//var needDisconnect bool
	var res = OracleTaskResult{}
	if err := r.connect(userName, userPass, connStr); err != nil {
		res.StatusCode, res.Content /*needDisconnect*/, _ = packError(err)
		// Формируем дамп до закрытия соединения, чтобы получить корректный запрос из последнего шага
		r.dumpError(userName, connStr, dumpErrorFileName, err)

		//Если произошла ошибка, всегда закрываем соединение с БД
		r.disconnect()

		res.Duration = int64(time.Since(bg) / time.Second)
		func() {
			r.mt.Lock()
			defer r.mt.Unlock()
			r.logErrorsNum++
		}()
		return res
	}

	if err := r.run(&res, paramStoreProc, beforeScript, afterScript, documentTable,
		cgiEnv, procName, urlParams, reqFiles); err != nil {
		res.StatusCode, res.Content /*needDisconnect*/, _ = packError(err)
		// Формируем дамп до закрытия соединения, чтобы получить корректный запрос из последнего шага
		r.dumpError(userName, connStr, dumpErrorFileName, err)

		//Если произошла ошибка, всегда закрываем соединение с БД
		r.disconnect()

		//		if needDisconnect {
		//			//Если произошла ошибка, всегда закрываем соединение с БД
		//			r.disconnect()
		//		}
		res.Duration = int64(time.Since(bg) / time.Second)
		func() {
			r.mt.Lock()
			defer r.mt.Unlock()
			r.logErrorsNum++
		}()
		return res
		//	} else {
		//		r.dumpError("!!!"+userName, connStr, dumpErrorFileName, errgo.New("TEST"))
	}
	res.StatusCode = http.StatusOK
	res.Duration = int64(time.Since(bg) / time.Second)
	return res
}

func (r *oracleTasker) connect(username, userpass, connstr string) (err error) {

	if (r.conn == nil) || (r.connUserName != username) || (r.connUserPass != userpass) || (r.connStr != connstr) {
		r.disconnect()

		return func() error {
			r.openStep(stepConnectNum, "connect")
			defer r.closeStep(stepConnectNum)
			r.setStepInfo(stepConnectNum, "connect", "connect", false)
			r.conn, err = oracle.NewConnection(username, userpass, connstr, false)
			if err != nil {
				// Если выходим с ошибкой, то в вызывающей процедуре будет вызван disconnect()
				return err
			}

			r.connUserName = username
			r.connUserPass = userpass
			r.connStr = connstr
			// Соединение с БД прошло успешно.
			if err = r.evalSessionID(); err != nil {
				// Если выходим с ошибкой, то в вызывающей процедуре будет вызван disconnect()
				return err
			}
			r.setStepInfo(stepConnectNum, "connect", "connect", true)
			return nil
		}()
	}
	if !r.conn.IsConnected() {
		panic("Сюда приходить никогда не должны !!!")
	}

	return nil
}

func (r *oracleTasker) disconnect() (err error) {
	if r.conn != nil {
		if r.conn.IsConnected() {
			r.openStep(stepDisconnectNum, "disconnect")
			r.setStepInfo(stepDisconnectNum, "disconnect", "disconnect", false)

			r.conn.Close()

			r.setStepInfo(stepDisconnectNum, "disconnect", "disconnect", true)
			r.closeStep(stepDisconnectNum)

		} else {
			// Очистка в случае неудачного Logon
			r.conn.Free(true)

		}
		r.mt.Lock()
		r.conn = nil
		r.connUserName = ""
		r.connUserPass = ""
		r.connStr = ""
		r.sessID = ""
		r.mt.Unlock()
	}
	return nil
}

func (r *oracleTasker) evalSessionID() error {
	r.openStep(stepEvalSid, "evalSessionID")

	cur := r.conn.NewCursor()
	defer func() { cur.Close(); r.closeStep(stepEvalSid) }()

	r.sessID = ""
	stepStm := r.stmEvalSessionID
	r.setStepInfo(stepEvalSid, stepStm, stepStm, false)

	err := cur.Execute(stepStm, nil, nil)
	if err != nil {
		return err
	}
	row, err1 := cur.FetchOne()
	if err1 != nil {
		return err1
	}
	r.sessID = row[0].(string)
	r.setStepInfo(stepEvalSid, stepStm, stepStm, true)
	return err
}

func (r *oracleTasker) run(res *OracleTaskResult, paramStoreProc, beforeScript, afterScript, documentTable string,
	cgiEnv map[string]string, procName string, urlParams url.Values, reqFiles *mltpart.Form) error {

	const (
		initParams = `
  l_num_params := :num_params;
  l_param_name := :param_name;
  l_param_val := :param_val;
  l_num_ext_params := :num_ext_params;
  l_ext_param_name := :ext_param_name;
  l_ext_param_val := :ext_param_val;
  l_package_name := :package_name;
`
	)

	var (
		numParamsVar        *oracle.Variable
		paramNameVar        *oracle.Variable
		paramValVar         *oracle.Variable
		ContentTypeVar      *oracle.Variable
		ContentLengthVar    *oracle.Variable
		CustomHeadersVar    *oracle.Variable
		rcVar               *oracle.Variable
		contentVar          *oracle.Variable
		bNextChunkExistsVar *oracle.Variable
		lobVar              *oracle.Variable
		sqlErrCodeVar       *oracle.Variable
		sqlErrMVar          *oracle.Variable
		sqlErrTraceVar      *oracle.Variable
	)
	var (
		stmExecDeclarePart bytes.Buffer
		stmShowDeclarePart bytes.Buffer

		stmExecSetPart bytes.Buffer
		stmShowSetPart bytes.Buffer

		stmExecProcParams     bytes.Buffer
		stmShowProcParams     bytes.Buffer
		stmExecStoreInContext bytes.Buffer
		stmShowStoreInContext bytes.Buffer
	)

	r.openStep(stepRunNum, "run")
	defer r.closeStep(stepRunNum)

	cur := r.conn.NewCursor()
	defer cur.Close()

	stmExecSetPart.WriteString(initParams)

	procNameParts := strings.Split(procName, "/")
	if len(procNameParts) > 1 {
		cgiEnv["X-APEX-BASE"] = "/" + procNameParts[0]
	}

	numParams := int32(len(cgiEnv))

	var (
		err             error
		paramNameMaxLen int
		paramValMaxLen  int
	)
	for key, val := range cgiEnv {
		if len(key) > paramNameMaxLen {
			paramNameMaxLen = len(key)
		}
		if len(val) > paramValMaxLen {
			paramValMaxLen = len(val)
		}
	}

	if numParamsVar, err = cur.NewVar(&numParams); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", numParams, numParams, err)
	}

	if paramNameVar, err = cur.NewVariable(uint(numParams), oracle.StringVarType, uint(paramNameMaxLen)); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "paramName", "string", err)
	}

	if paramValVar, err = cur.NewVariable(uint(numParams), oracle.StringVarType, uint(paramValMaxLen)); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "paramVal", "string", err)
	}

	stmShowSetPart.WriteString(fmt.Sprintf("  l_num_params := %d;\n", numParams))

	var cgiEnvKeys []string
	for key := range cgiEnv {
		cgiEnvKeys = append(cgiEnvKeys, key)
	}
	sort.Strings(cgiEnvKeys)

	//i := uint(0)
	for key := range cgiEnvKeys {
		paramNameVar.SetValue(uint(key), cgiEnvKeys[key])
		paramValVar.SetValue(uint(key), cgiEnv[cgiEnvKeys[key]])

		stmShowSetPart.WriteString(fmt.Sprintf("  l_param_name(%d) := '%s';\n", key+1, cgiEnvKeys[key]))
		stmShowSetPart.WriteString(fmt.Sprintf("  l_param_val(%d) := '%s';\n", key+1, cgiEnv[cgiEnvKeys[key]]))
		//i++
	}

	if ContentTypeVar, err = cur.NewVariable(0, oracle.StringVarType, 1024); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "ContentType", "varchar2(32767)", err)
	}

	if ContentLengthVar, err = cur.NewVariable(0, oracle.Int32VarType, 0); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "ContentLength", "number", err)
	}

	if CustomHeadersVar, err = cur.NewVariable(0, oracle.StringVarType, 32767); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "CustomHeaders", "varchar2(32767)", err)
	}

	if rcVar, err = cur.NewVariable(0, oracle.Int32VarType, 0); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "rc__", "number", err)
	}

	if contentVar, err = cur.NewVariable(0, oracle.StringVarType, 32767); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "content__", "varchar2(32767)", err)
	}

	if bNextChunkExistsVar, err = cur.NewVariable(0, oracle.Int32VarType, 0); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "bNextChunkExists", "number", err)
	}

	if lobVar, err = cur.NewVariable(0, oracle.BlobVarType, 0); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "BlobVarType", "BlobVarType", err)
	}

	if sqlErrCodeVar, err = cur.NewVariable(0, oracle.Int32VarType, 0); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "sqlErrCode", "number", err)
	}

	if sqlErrMVar, err = cur.NewVariable(0, oracle.StringVarType, 32767); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "sqlErrM", "varchar2(32767)", err)
	}

	if sqlErrTraceVar, err = cur.NewVariable(0, oracle.StringVarType, 32767); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "sqlErrTrace", "varchar2(32767)", err)
	}

	sqlParams := map[string]interface{}{"num_params": numParamsVar,
		"param_name":       paramNameVar,
		"param_val":        paramValVar,
		"ContentType":      ContentTypeVar,
		"ContentLength":    ContentLengthVar,
		"CustomHeaders":    CustomHeadersVar,
		"rc__":             rcVar,
		"content__":        contentVar,
		"lob__":            lobVar,
		"bNextChunkExists": bNextChunkExistsVar,
		"sqlerrcode":       sqlErrCodeVar,
		"sqlerrm":          sqlErrMVar,
		"sqlerrtrace":      sqlErrTraceVar}

	var (
		extParamName        []interface{}
		extParamValue       []interface{}
		extParamNameMaxLen  int
		extParamValueMaxLen int
	)

	if len(procNameParts) > 1 {
		//Получение ресурсов новым способом
		procName = "get_resource"

		module := procNameParts[0] + "/" + procNameParts[1] + "/"
		method := cgiEnv["REQUEST_METHOD"]
		url := strings.Join(procNameParts[2:], "/")

		if err := prepareStringParam(cur, sqlParams, "amodule", []string{module},
			"varchar2", "",
			&stmExecDeclarePart, &stmShowDeclarePart,
			&stmExecSetPart, &stmShowSetPart,
			&stmExecProcParams, &stmShowProcParams,
			&stmExecStoreInContext, &stmShowStoreInContext); err != nil {
			return err
		}
		if err := prepareStringParam(cur, sqlParams, "amethod", []string{method},
			"varchar2", "",
			&stmExecDeclarePart, &stmShowDeclarePart,
			&stmExecSetPart, &stmShowSetPart,
			&stmExecProcParams, &stmShowProcParams,
			&stmExecStoreInContext, &stmShowStoreInContext); err != nil {
			return err
		}
		if err := prepareStringParam(cur, sqlParams, "aurl", []string{url},
			"varchar2", "",
			&stmExecDeclarePart, &stmShowDeclarePart,
			&stmExecSetPart, &stmShowSetPart,
			&stmExecProcParams, &stmShowProcParams,
			&stmExecStoreInContext, &stmShowStoreInContext); err != nil {
			return err
		}
		pkgName := ""
		var pnVar *oracle.Variable
		pnVar, err = cur.NewVariable(0, oracle.StringVarType, 80)
		if err != nil {
			return errgo.Newf("error creating variable for %s(%T): %s", "package_name", "varchar2", err)
		}
		pnVar.SetValue(0, pkgName)
		sqlParams["package_name"] = pnVar

	} else {

		err := func() error {
			r.openStep(stepDescribeNum, "Describe")
			defer r.closeStep(stepDescribeNum)
			return Describe(r.conn, r.connStr, procName)

		}()
		if err != nil {
			return err
		}

		if reqFiles != nil {
			for paramName, paramValue := range reqFiles.File {
				fileName, err := r.saveFile(paramStoreProc, beforeScript, afterScript, documentTable,
					cgiEnv, urlParams, paramValue)
				if err != nil {
					return err
				}

				paramType, paramTypeName, _ := ArgumentInfo(r.connStr, procName, paramName)

				err = prepareParam(cur, sqlParams,
					paramName, fileName,
					paramType, paramTypeName,
					paramStoreProc,
					&stmExecDeclarePart, &stmShowDeclarePart,
					&stmExecSetPart, &stmShowSetPart,
					&stmExecProcParams, &stmShowProcParams,
					&stmExecStoreInContext, &stmShowStoreInContext)
				if err != nil {
					return err
				}

				extParamName = append(extParamName, paramName)
				extParamValue = append(extParamValue, fileName[0])

				if len(fileName[0]) > extParamValueMaxLen {
					extParamValueMaxLen = len(fileName[0])
				}
			}
		}

		for paramName, paramValue := range urlParams {
			paramName = strings.Trim(paramName, " ")

			if paramName != "" {
				paramType, paramTypeName, _ := ArgumentInfo(r.connStr, procName, paramName)

				err = prepareParam(cur, sqlParams,
					paramName, paramValue,
					paramType, paramTypeName,
					paramStoreProc,
					&stmExecDeclarePart, &stmShowDeclarePart,
					&stmExecSetPart, &stmShowSetPart,
					&stmExecProcParams, &stmShowProcParams,
					&stmExecStoreInContext, &stmShowStoreInContext)
				if err != nil {
					return err
				}

				extParamName = append(extParamName, paramName)
				extParamValue = append(extParamValue, paramValue[0])

				if len(paramName) > extParamNameMaxLen {
					extParamNameMaxLen = len(paramName)
				}

				if len(paramValue[0]) > extParamValueMaxLen {
					extParamValueMaxLen = len(paramValue[0])
				}
			}
		}
		var pkgName string
		_, pkgName, err = ProcedureInfo(r.connStr, procName)
		if err != nil {
			return err
		}

		var pnVar *oracle.Variable
		pnVar, err = cur.NewVariable(0, oracle.StringVarType, 80)
		if err != nil {
			return errgo.Newf("error creating variable for %s(%T): %s", "package_name", "varchar2", err)
		}
		pnVar.SetValue(0, pkgName)
		sqlParams["package_name"] = pnVar

		//	if reqFiles != nil {
		//		for paramName, paramValue := range reqFiles.File {
		//			fileName, err := r.saveFile(paramStoreProc, beforeScript, afterScript, documentTable,
		//				cgiEnv, urlParams, paramValue)
		//			if err != nil {
		//				return err
		//			}
		//			paramType, paramTypeName, _ := ArgumentInfo(r.connStr, procName, paramName)

		//			fmt.Println("fileName = ", fileName)
		//			fmt.Println("paramType = ", paramType)
		//			fmt.Println("paramTypeName = ", paramTypeName)

		//			err = prepareParam(cur, sqlParams,
		//				paramName, fileName,
		//				paramType, paramTypeName,
		//				paramStoreProc,
		//				&stmExecDeclarePart, &stmShowDeclarePart,
		//				&stmExecSetPart, &stmShowSetPart,
		//				&stmExecProcParams, &stmShowProcParams,
		//				&stmExecStoreInContext, &stmShowStoreInContext)
		//			if err != nil {
		//				return err
		//			}

		//			extParamName = append(extParamName, paramName)
		//			extParamValue = append(extParamValue, fileName[0])

		//			if len(fileName[0]) > extParamValueMaxLen {
		//				extParamValueMaxLen = len(fileName[0])
		//			}

		//		}
		//	}
	}
	stmExecSetPart.WriteString(fmt.Sprintf("  l_num_ext_params := %d;\n", int32(len(extParamName))))
	stmShowSetPart.WriteString(fmt.Sprintf("  l_num_ext_params := %d;\n", int32(len(extParamName))))
	for key, val := range extParamName {
		//FIXME Заменить текстовую константу на использование bind-переменной
		s, _ := val.(string)
		stmExecSetPart.WriteString(fmt.Sprintf("  l_ext_param_name(%d) := '%s';\n", key+1, strings.ToUpper(s)))
		stmShowSetPart.WriteString(fmt.Sprintf("  l_ext_param_name(%d) := '%s';\n", key+1, strings.ToUpper(s)))
	}

	for key, val := range extParamValue {
		s, _ := val.(string)
		stmExecSetPart.WriteString(fmt.Sprintf("  l_ext_param_val(%d) := '%s';\n", key+1, strings.Replace(s, "'", "''", -1)))
		stmShowSetPart.WriteString(fmt.Sprintf("  l_ext_param_val(%d) := '%s';\n", key+1, strings.Replace(s, "'", "''", -1)))
	}

	if sqlParams["ext_param_name"], err = cur.NewArrayVar(oracle.StringVarType, extParamName, uint(extParamNameMaxLen)); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "ext_param_name", "varchar2", err)
	}

	if sqlParams["ext_param_val"], err = cur.NewArrayVar(oracle.StringVarType, extParamValue, uint(extParamValueMaxLen)); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "ext_param_val", "varchar2", err)
	}

	epn := int32(len(extParamName))
	if sqlParams["num_ext_params"], err = cur.NewVar(&epn); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "num_ext_params", "number", err)
	}

	stepStm := fmt.Sprintf(r.stmMain, stmExecDeclarePart.String(), stmExecSetPart.String(), beforeScript, stmExecStoreInContext.String(), procName, stmExecProcParams.String(), afterScript)
	stepStmForShowing := fmt.Sprintf(r.stmMain, stmShowDeclarePart.String(), stmShowSetPart.String(), beforeScript, stmShowStoreInContext.String(), procName, stmShowProcParams.String(), afterScript)
	stepStmParams := sqlParams

	r.setStepInfo(stepRunNum, stepStm, stepStmForShowing, false)

	if err := cur.Execute(stepStm, nil, stepStmParams); err != nil {
		//		e := UnMask(err)
		//		if e != nil {
		//			if e.Code == 6550 {
		//				for k, v := range stepStmParams {
		//					v1 := v.(*oracle.Variable)
		//					fmt.Println(k, v1.IsArray(), v1.AllocatedElements(), v1.ArrayLength(), v1.Size(), v1.String())
		//				}
		//			}
		//			os.Exit(-1)
		//		}
		return err
	}

	sqlErrCode, err := sqlErrCodeVar.GetValue(0)
	if err != nil {
		return err
	}

	if sqlErrCode.(int32) != 0 {
		sqlErrM, err := sqlErrMVar.GetValue(0)
		if err != nil {
			return err
		}
		sqlErrTrace, err := sqlErrTraceVar.GetValue(0)
		if err != nil {
			return err
		}
		return oracle.NewErrorAt(int(sqlErrCode.(int32)), sqlErrM.(string), sqlErrTrace.(string))
	}

	ct, err := ContentTypeVar.GetValue(0)
	if err != nil {
		return err
	}

	contentType := ""
	if ct != nil {
		contentType = ct.(string)
	}

	ch, err := CustomHeadersVar.GetValue(0)
	if err != nil {
		return err
	}
	if ch != nil {
		res.Headers = parseHeaders(ch.(string))
	}

	//ContentType передается через дополнительные заголовки - ЕКБ
	for k, v := range res.Headers {
		if strings.ToLower(k) == "content-type" {
			contentType = strings.Join(v, ";")
			res.Headers.Del(k)
		}
	}

	res.ContentType = contentType

	rc, err := rcVar.GetValue(0)
	if err != nil {
		return err
	}

	switch rc.(int32) {
	case 0:
		{
			// Oracle возвращает данные ВСЕГДА в UTF-8
			data, err := contentVar.GetValue(0)
			if err != nil {
				return err
			}
			if data == nil {
				return nil
			}
			// Oracle возвращает данные ВСЕГДА в UTF-8
			res.Content = append(res.Content, []byte(addCR(data.(string)))...)
			bNextChunkExists, err := bNextChunkExistsVar.GetValue(0)
			if err != nil {
				return err
			}
			if bNextChunkExists.(int32) != 0 {
				r.getRestChunks(res)
			}

			//FIXME - Убрать костыль после того. как принудительная установка будет удалена из кода на PL/SQL
			//В коде на PL/SQL встречаются места, где принудительно устанавливается charset.
			//Поскольку библиотека получает данные в UTF-8, приходиться менять/удалять charset как в заголовках, так и в теле ответа
			res.ContentType, _, _ = fixContentType(contentType)
			res.Content = fixMeta(res.Content)
			if res.ContentType == "" {
				res.ContentType = http.DetectContentType(res.Content)
			}
		}
	case 1:
		{
			data, err := lobVar.GetValue(0)
			if err != nil {
				return err
			}
			ext, ok := data.(*oracle.ExternalLobVar)
			if !ok {
				return errgo.Newf("data is not *ExternalLobVar, but %T", data)
			}
			if ext != nil {
				size, err := ext.Size(false)
				if err != nil {
					fmt.Println("size error: ", err)
				}
				if size != 0 {
					//Было так
					res.Content, err = ext.ReadAll()
					if err != nil {
						return err
					}
					if res.ContentType == "" {
						res.ContentType = http.DetectContentType(res.Content)
					}

					//					//Костыль
					//					if fixedFlag {
					//						//Для случая, когда требуется перекодировка
					//						if origCharset == "" {
					//							origCharset = "windows-1251"
					//						}

					//						fmt.Println("origCharset = ", origCharset)

					//						if e, _ := charset.Lookup(origCharset); e != encoding.Nop {
					//							res.Content, err = ioutil.ReadAll(transform.NewReader(bytes.NewReader(res.Content), e.NewDecoder()))
					//							if err != nil {
					//								return err
					//							}
					//						}
					//					}
				}
			}

		}
	}
	if res.ContentType == "" /*&& (len(res.Content) > 0)*/ {
		res.ContentType = "text/html"
	}

	r.setStepInfo(stepRunNum, stepStm, stepStmForShowing, true)
	return nil
}

func (r *oracleTasker) getRestChunks(res *OracleTaskResult) error {
	var (
		err                 error
		bNextChunkExists    int32
		DataVar             *oracle.Variable
		bNextChunkExistsVar *oracle.Variable
		sqlErrCodeVar       *oracle.Variable
		sqlErrMVar          *oracle.Variable
		sqlErrTraceVar      *oracle.Variable
	)
	r.openStep(stepChunkGetNum, "getRestChunks")
	cur := r.conn.NewCursor()
	defer func() { cur.Close(); r.closeStep(stepChunkGetNum) }()

	if DataVar, err = cur.NewVariable(0, oracle.StringVarType, 32767); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "Data", "string", err)
	}

	if bNextChunkExistsVar, err = cur.NewVar(&bNextChunkExists); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", bNextChunkExists, bNextChunkExists, err)
	}
	if sqlErrCodeVar, err = cur.NewVariable(0, oracle.Int32VarType, 0); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "sqlErrCode", "number", err)
	}

	if sqlErrMVar, err = cur.NewVariable(0, oracle.StringVarType, 32767); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "sqlErrM", "varchar2(32767)", err)
	}

	if sqlErrTraceVar, err = cur.NewVariable(0, oracle.StringVarType, 32767); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "sqlErrTrace", "varchar2(32767)", err)
	}

	stepStm := r.stmGetRestChunk
	stepStmParams := map[string]interface{}{
		"Data":             DataVar,
		"bNextChunkExists": bNextChunkExistsVar,
		"sqlerrcode":       sqlErrCodeVar,
		"sqlerrm":          sqlErrMVar,
		"sqlerrtrace":      sqlErrTraceVar,
	}
	r.setStepInfo(stepChunkGetNum, stepStm, stepStm, true)
	bNextChunkExists = 1

	for bNextChunkExists != 0 {
		if err := cur.Execute(stepStm, nil, stepStmParams); err != nil {
			return err
		}
		sqlErrCode, err := sqlErrCodeVar.GetValue(0)
		if err != nil {
			return err
		}

		if sqlErrCode.(int32) != 0 {
			sqlErrM, err := sqlErrMVar.GetValue(0)
			if err != nil {
				return err
			}
			sqlErrTrace, err := sqlErrTraceVar.GetValue(0)
			if err != nil {
				return err
			}
			return oracle.NewErrorAt(int(sqlErrCode.(int32)), sqlErrM.(string), sqlErrTrace.(string))
		}
		data, err := DataVar.GetValue(0)
		if err != nil {
			return err
		}
		// Oracle возвращает данные ВСЕГДА в UTF-8
		res.Content = append(res.Content, []byte(addCR(data.(string)))...)
	}
	r.setStepInfo(stepChunkGetNum, stepStm, stepStm, true)
	return nil
}

func (r *oracleTasker) saveFile(paramStoreProc, beforeScript, afterScript, documentTable string,
	cgiEnv map[string]string, urlParams url.Values, fileHeaders []*mltpart.FileHeader) ([]string, error) {
	fileNames := make([]string, len(fileHeaders))
	for i, fileHeader := range fileHeaders {
		//Если Header == nil, значит заголовок сделан из значения параметра, переданного как строковая
		//просто добавляем в список имен
		if fileHeader.Header == nil {
			fileNames[i] = fileHeader.Filename
			continue
		}
		//_, fileNames[i] = filepath.Split(fileHeader.Filename)
		fileNames[i] = ExtractFileName(fileHeader.Header.Get("Content-Disposition"))

		fileReader, err := fileHeader.Open()
		if err != nil {
			return nil, err
		}
		fileContent, err := ioutil.ReadAll(fileReader)
		if err != nil {
			return nil, err
		}
		//_ = fileContent
		fileContentType := fileHeader.Header.Get("Content-Type")
		fileNames[i], err = r.saveFileToDB(paramStoreProc, beforeScript, afterScript, documentTable,
			cgiEnv, urlParams, fileNames[i], fileHeader.LastArg, fileContentType, fileContentType, fileContent)
		if err != nil {
			return nil, err
		}
	}
	return fileNames, nil
}

func (r *oracleTasker) saveFileToDB(paramStoreProc, beforeScript, afterScript, documentTable string,
	cgiEnv map[string]string, urlParams url.Values, fName, fItem, fMime, fContentType string, fContent []byte) (string, error) {

	r.openStep(stepSaveFileToDBNum, "saveFileToDB")
	cur := r.conn.NewCursor()
	defer func() { cur.Close(); r.closeStep(stepSaveFileToDBNum) }()

	numParams := int32(len(cgiEnv))

	var (
		paramNameMaxLen int
		paramValMaxLen  int
	)
	for key, val := range cgiEnv {
		if len(key) > paramNameMaxLen {
			paramNameMaxLen = len(key)
		}
		if len(val) > paramValMaxLen {
			paramValMaxLen = len(val)
		}
	}
	numParamsVar, err := cur.NewVar(&numParams)
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", numParams, numParams, err)
	}
	paramNameVar, err := cur.NewVariable(uint(numParams), oracle.StringVarType, uint(paramNameMaxLen))
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", "paramName", "string", err)
	}
	paramValVar, err := cur.NewVariable(uint(numParams), oracle.StringVarType, uint(paramValMaxLen))
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", "paramVal", "string", err)
	}

	i := uint(0)
	for key, val := range cgiEnv {
		paramNameVar.SetValue(i, key)
		paramValVar.SetValue(i, val)
		i++
	}
	nameVar, err := cur.NewVar(&fName)
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", fName, fName, err)
	}

	mimeVar, err := cur.NewVar(&fMime)
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", fMime, fMime, err)
	}

	ContentTypeVar, err := cur.NewVar(&fContentType)
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", fContentType, fContentType, err)
	}

	docSize := len(fContent)
	docSizeVar, err := cur.NewVar(&docSize)
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", docSize, docSize, err)
	}

	lobVar, err := cur.NewVariable(0, oracle.BlobVarType, uint(docSize))
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(lob): %s", "lob", err)
	}

	if err := lobVar.SetValue(0, fContent); err != nil {
		return "", errgo.Newf("error setting variable for %s(lob): %s", "lob", err)
	}

	itemID := fItem

	applicationID := urlParams.Get("p_flow_id")
	pageID := urlParams.Get("p_flow_step_id")
	sessionID := urlParams.Get("p_instance")
	request := urlParams.Get("p_request")

	itemIDVar, err := cur.NewVar(&itemID)
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", itemID, itemID, err)
	}
	applicationIDVar, err := cur.NewVar(&applicationID)
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", applicationID, applicationID, err)
	}

	pageIDVar, err := cur.NewVar(&pageID)
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", pageID, pageID, err)
	}
	sessionIDVar, err := cur.NewVar(&sessionID)
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", sessionID, sessionID, err)
	}
	requestVar, err := cur.NewVar(&request)
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", request, request, err)
	}

	retName := ""
	retNameVar, err := cur.NewVar(&retName)
	if err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", retName, retName, err)
	}

	var (
		sqlErrCodeVar  *oracle.Variable
		sqlErrMVar     *oracle.Variable
		sqlErrTraceVar *oracle.Variable
	)
	if sqlErrCodeVar, err = cur.NewVariable(0, oracle.Int32VarType, 0); err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", "sqlErrCode", "number", err)
	}

	if sqlErrMVar, err = cur.NewVariable(0, oracle.StringVarType, 32767); err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", "sqlErrM", "varchar2(32767)", err)
	}

	if sqlErrTraceVar, err = cur.NewVariable(0, oracle.StringVarType, 32767); err != nil {
		return "", errgo.Newf("error creating variable for %s(%T): %s", "sqlErrTrace", "varchar2(32767)", err)
	}
	//fmt.Println(fmt.Sprintf(r.stmFileUpload, task.beforeScript, task.documentTable))

	stepStm := fmt.Sprintf(r.stmFileUpload, beforeScript, documentTable)
	stepStmParams := map[string]interface{}{"num_params": numParamsVar,
		"param_name":     paramNameVar,
		"param_val":      paramValVar,
		"name":           nameVar,
		"mime_type":      mimeVar,
		"doc_size":       docSizeVar,
		"content_type":   ContentTypeVar,
		"lob":            lobVar,
		"item_id":        itemIDVar,
		"application_id": applicationIDVar,
		"page_id":        pageIDVar,
		"session_id":     sessionIDVar,
		"request":        requestVar,
		"ret_name":       retNameVar,
		"sqlerrcode":     sqlErrCodeVar,
		"sqlerrm":        sqlErrMVar,
		"sqlerrtrace":    sqlErrTraceVar}

	r.setStepInfo(stepSaveFileToDBNum, stepStm, stepStm, false)

	if err := cur.Execute(stepStm, nil, stepStmParams); err != nil {
		return "", err
	}
	sqlErrCode, err := sqlErrCodeVar.GetValue(0)
	if err != nil {
		return "", err
	}

	if sqlErrCode.(int32) != 0 {
		sqlErrM, err := sqlErrMVar.GetValue(0)
		if err != nil {
			return "", err
		}
		sqlErrTrace, err := sqlErrTraceVar.GetValue(0)
		if err != nil {
			return "", err
		}
		return "", oracle.NewErrorAt(int(sqlErrCode.(int32)), sqlErrM.(string), sqlErrTrace.(string))
	}
	ret, e := retNameVar.GetValue(0)
	if e != nil {
		return "", err
	}

	r.setStepInfo(stepSaveFileToDBNum, stepStm, stepStm, true)
	return ret.(string), nil
}

func (r *oracleTasker) openStep(stepNum int, stepType string) {
	r.mt.Lock()
	defer r.mt.Unlock()

	stepName := fmt.Sprintf("%03d - %s", stepNum, stepType)
	intf := stepsFree.Get()
	step := intf.(*oracleTaskerStep)
	step.stepID = stepNum
	step.stepName = stepName
	step.stepBg = time.Now()
	step.stepFn = time.Time{}
	step.stepStm = ""
	step.stepSuccess = false
	r.logSteps[stepNum] = step
}

func (r *oracleTasker) closeStep(stepNum int) {
	r.mt.Lock()
	defer r.mt.Unlock()
	step := r.logSteps[stepNum]
	step.stepFn = time.Now()
	r.logSteps[stepNum] = step
}
func (r *oracleTasker) setStepInfo(stepNum int, stepStm, stepStmForShowning string, stepSuccess bool) {
	r.mt.Lock()
	defer r.mt.Unlock()
	step := r.logSteps[stepNum]
	step.stepStm = stepStm
	step.stepStmForShowning = stepStmForShowning
	step.stepSuccess = stepSuccess
	r.logSteps[stepNum] = step
}

func (r *oracleTasker) Break() error {
	r.mt.Lock()
	defer r.mt.Unlock()
	// Прерываем выполнение текущей сессии.
	// Используем параметры сохраненные подключения
	if !r.stateIsWorking {
		//Выполнение уже завершилось. Нечего прерывать
		return nil
	}
	if r.conn == nil {
		//Выполнение еще не начиналось. Нечего прерывать
		return nil
	}
	if !r.conn.IsConnected() {
		//Выполнение еще не начиналось. Нечего прерывать
		return nil
	}

	if r.sessID == "" {
		return errgo.Newf("Отсутствует информация о сессии")
	}

	return killSession(r.stmKillSession, r.connUserName, r.connUserPass, r.connStr, r.sessID)
}

func killSession(stm, username, password, sid, sessionID string) error {
	conn, err := oracle.NewConnection(username, password, sid, false)
	if err != nil {
		return err
	}
	// Соединение с БД прошло успешно.
	defer conn.Close()

	cur := conn.NewCursor()
	defer cur.Close()

	sesVar, err := cur.NewVariable(0, oracle.StringVarType, 40)
	if err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "sessId", sessionID, err)
	}
	sesVar.SetValue(0, sessionID)

	retMsg, err := cur.NewVariable(0, oracle.StringVarType, 32767)
	if err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "retMsg", "varchar2", err)
	}
	retVar, err := cur.NewVariable(0, oracle.Int32VarType, 0)
	if err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", "retVar", "number", err)
	}

	if err := cur.Execute(stm, nil, map[string]interface{}{"sess_id": sesVar, "ret": retVar, "out_err_msg": retMsg}); err != nil {
		return err
	}

	ret, err := retVar.GetValue(0)
	if err != nil {
		return err
	}
	if ret.(int32) != 1 {
		msg, err := retMsg.GetValue(0)
		if err != nil {
			return err
		}
		return errgo.New(msg.(string))
	}

	return nil
}

func (r *oracleTasker) dumpError(userName, connStr, dumpErrorFileName string, err error) {
	stm, stmShow := r.lastStms()
	var buf bytes.Buffer
	// BOM
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	buf.WriteString(fmt.Sprintf("Имя пользователя : %s\r\n", userName))
	buf.WriteString(fmt.Sprintf("Строка соединения : %s\r\n", connStr))
	buf.WriteString(fmt.Sprintf("Дата и время возникновения : %s\r\n", time.Now().Format(time.RFC1123Z)))
	buf.WriteString("******* Текст SQL  ********************************\r\n")
	buf.WriteString(strings.Replace(stm, "\n", "\r\n", -1) + "\r\n")
	buf.WriteString("******* Текст SQL закончен ************************\r\n")
	buf.WriteString("\r\n")
	buf.WriteString("******* Текст ошибки  *****************************\r\n")
	buf.WriteString(strings.Replace(err.Error(), "\n", "\r\n", -1) + "\r\n")
	buf.WriteString("******* Текст ошибки закончен *********************\r\n")
	buf.WriteString("\r\n")
	buf.WriteString("******* Текст SQL с параметрами *******************\r\n")
	buf.WriteString(strings.Replace(stmShow, "\n", "\r\n", -1) + "\r\n")
	buf.WriteString("******* Текст SQL с параметрами закончен **********\r\n")
	dir, _ := filepath.Split(dumpErrorFileName)
	os.MkdirAll(dir, os.ModeDir)
	ioutil.WriteFile(dumpErrorFileName, buf.Bytes(), 0644)
}

func (r *oracleTasker) lastStms() (string, string) {
	r.mt.Lock()
	defer r.mt.Unlock()

	var keys []int
	for k := range r.logSteps {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return "", ""
	}

	sort.Sort(sort.Reverse(sort.IntSlice(keys)))
	step, ok := r.logSteps[keys[0]]
	if !ok {
		return "", ""
	}
	return step.stepStm, step.stepStmForShowning
}

func packError(err error) (int, []byte, bool) {
	oraErr := UnMask(err)
	if oraErr != nil {
		switch oraErr.Code {
		case 28, 31:
			return StatusRequestWasInterrupted, []byte(""), true
		case 1017:
			return StatusInvalidUsernameOrPassword, []byte(""), true
		case 1031:
			return StatusInsufficientPrivileges, []byte(""), true
		case 28000:
			return StatusAccountIsLocked, []byte(""), true
		case 6564:
			return http.StatusNotFound, []byte(""), false
		case 3113, 3114:
			return StatusErrorPage, []byte(errgo.Mask(err).Error()), true
		default:
			return StatusErrorPage, []byte(errgo.Mask(err).Error()), false
		}
	}
	return StatusErrorPage, []byte(errgo.Mask(err).Error()), false

}

func prepareParam(
	cur *oracle.Cursor, params map[string]interface{},
	paramName string, paramValue []string,
	paramType int32, paramTypeName string,
	paramStoreProc string,
	stmExecDeclarePart, stmShowDeclarePart,
	stmExecSetPart, stmShowSetPart,
	stmExecProcParams, stmShowProcParams,
	stmExecStoreInContext, stmShowStoreInContext *bytes.Buffer,
	/*
		paramsForCall, paramsForStore bytes.Buffer*/) error {
	var (
		lVar *oracle.Variable
		err  error
	)

	switch paramType {
	case oString:
		{
			return prepareStringParam(cur, params, paramName, paramValue,
				paramTypeName, paramStoreProc,
				stmExecDeclarePart, stmShowDeclarePart,
				stmExecSetPart, stmShowSetPart,
				stmExecProcParams, stmShowProcParams,
				stmExecStoreInContext, stmShowStoreInContext)
			//			value := removeCR(paramValue[0])

			//			if lVar, err = cur.NewVariable(0, oracle.StringVarType, uint(len(value))); err != nil {
			//				return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
			//			}
			//			lVar.SetValue(0, value)

			//			params[paramName] = lVar

			//			// stmExecDeclarePart
			//			iLen := len(value)
			//			if iLen == 0 {
			//				// Для того, чтобы избежать ситуации VARCHAR2(0);
			//				iLen = iLen + 1
			//			}
			//			stmShowDeclarePart.WriteString(fmt.Sprintf("  l_%s %s(%d);\n", paramName, paramTypeName, iLen))
			//			//stmExecSetPart,
			//			stmShowSetPart.WriteString(fmt.Sprintf("  l_%s := '%s';\n", paramName, strings.Replace(value, "'", "''", -1)))
			//			// Вызов процедуры - Формирование строки с параметрами для вызова процедуры
			//			if stmExecProcParams.Len() != 0 {
			//				stmExecProcParams.WriteString(", ")
			//			}
			//			stmExecProcParams.WriteString(fmt.Sprintf("%s => :%s", paramName, paramName))

			//			// Отображение вызова процедуры - Формирование строки с параметрами для вызова процедуры
			//			if stmShowProcParams.Len() != 0 {
			//				stmShowProcParams.WriteString(", ")
			//			}
			//			stmShowProcParams.WriteString(fmt.Sprintf("%s => l_%s", paramName, paramName))

			//			// Добавление вызова сохранения параметра
			//			if paramStoreProc != "" {
			//				if lVar, err = cur.NewVar(&value); err != nil {
			//					return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
			//				}
			//				params[paramName+"#"] = lVar
			//				stmExecStoreInContext.WriteString(fmt.Sprintf("  %s('%s', :%s#);\n", paramStoreProc, strings.ToUpper(paramName), paramName))
			//				stmShowStoreInContext.WriteString(fmt.Sprintf("  %s('%s', l_%s);\n", paramStoreProc, strings.ToUpper(paramName), paramName))
			//			}
			//			return nil
		}
	case oNumber:
		{
			value := trimRightCRLF(paramValue[0])
			// Перешли на использование неявного преобразования для использования настроек из SESSION_INIT
			if lVar, err = cur.NewVariable(1, oracle.StringVarType, uint(len(value))); err != nil {
				return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
			}
			if value != "" {
				lVar.SetValue(0, value)
			}
			params[paramName] = lVar

			// stmExecDeclarePart
			stmShowDeclarePart.WriteString(fmt.Sprintf("  l_%s %s;\n", paramName, paramTypeName))
			//stmExecSetPart,
			stmShowSetPart.WriteString(fmt.Sprintf("  l_%s := %s;\n", paramName, value))
			// Вызов процедуры - Формирование строки с параметрами для вызова процедуры
			if stmExecProcParams.Len() != 0 {
				stmExecProcParams.WriteString(", ")
			}
			stmExecProcParams.WriteString(fmt.Sprintf("%s => :%s", paramName, paramName))

			// Отображение вызова процедуры - Формирование строки с параметрами для вызова процедуры
			if stmShowProcParams.Len() != 0 {
				stmShowProcParams.WriteString(", ")
			}
			stmShowProcParams.WriteString(fmt.Sprintf("%s => l_%s", paramName, paramName))

			// Добавление вызова сохранения параметра
			if paramStoreProc != "" {
				if lVar, err = cur.NewVar(&value); err != nil {
					return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
				}
				params[paramName+"#"] = lVar
				stmExecStoreInContext.WriteString(fmt.Sprintf("  %s('%s', :%s#);\n", paramStoreProc, strings.ToUpper(paramName), paramName))
				stmShowStoreInContext.WriteString(fmt.Sprintf("  %s('%s', l_%s);\n", paramStoreProc, strings.ToUpper(paramName), paramName))
			}
			return nil
		}
	case oInteger:
		{
			value := trimRightCRLF(paramValue[0])
			if lVar, err = cur.NewVar(&value); err != nil {
				return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
			}
			params[paramName] = lVar

			// stmExecDeclarePart
			stmShowDeclarePart.WriteString(fmt.Sprintf("  l_%s %s;\n", paramName, paramTypeName))
			//stmExecSetPart,
			stmShowSetPart.WriteString(fmt.Sprintf("  l_%s := %s;\n", paramName, value))
			// Вызов процедуры - Формирование строки с параметрами для вызова процедуры
			if stmExecProcParams.Len() != 0 {
				stmExecProcParams.WriteString(", ")
			}
			stmExecProcParams.WriteString(fmt.Sprintf("%s => :%s", paramName, paramName))

			// Отображение вызова процедуры - Формирование строки с параметрами для вызова процедуры
			if stmShowProcParams.Len() != 0 {
				stmShowProcParams.WriteString(", ")
			}
			stmShowProcParams.WriteString(fmt.Sprintf("%s => l_%s", paramName, paramName))

			// Добавление вызова сохранения параметра
			if paramStoreProc != "" {
				if lVar, err = cur.NewVar(&value); err != nil {
					return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
				}
				params[paramName+"#"] = lVar
				stmExecStoreInContext.WriteString(fmt.Sprintf("  %s('%s', :%s#);\n", paramStoreProc, strings.ToUpper(paramName), paramName))
				stmShowStoreInContext.WriteString(fmt.Sprintf("  %s('%s', l_%s);\n", paramStoreProc, strings.ToUpper(paramName), paramName))
			}
			return nil
		}

	case oDate:
		{
			value := trimRightCRLF(paramValue[0])
			if lVar, err = cur.NewVar(&value); err != nil {
				return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
			}
			params[paramName] = lVar

			// stmExecDeclarePart
			stmShowDeclarePart.WriteString(fmt.Sprintf("  l_%s %s;\n", paramName, paramTypeName))
			//stmExecSetPart,
			stmShowSetPart.WriteString(fmt.Sprintf("  l_%s := to_date('%s');\n", paramName, value))
			// Вызов процедуры - Формирование строки с параметрами для вызова процедуры
			if stmExecProcParams.Len() != 0 {
				stmExecProcParams.WriteString(", ")
			}
			stmExecProcParams.WriteString(fmt.Sprintf("%s => :%s", paramName, paramName))
			// Отображение вызова процедуры - Формирование строки с параметрами для вызова процедуры
			if stmShowProcParams.Len() != 0 {
				stmShowProcParams.WriteString(", ")
			}
			stmShowProcParams.WriteString(fmt.Sprintf("%s => l_%s", paramName, paramName))

			// Добавление вызова сохранения параметра
			if paramStoreProc != "" {
				if lVar, err = cur.NewVar(&value); err != nil {
					return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
				}
				params[paramName+"#"] = lVar
				stmExecStoreInContext.WriteString(fmt.Sprintf("  %s('%s', :%s#);\n", paramStoreProc, strings.ToUpper(paramName), paramName))
				stmShowStoreInContext.WriteString(fmt.Sprintf("  %s('%s', l_%s);\n", paramStoreProc, strings.ToUpper(paramName), paramName))
			}
			return nil
		}
	case oBoolean:
		{
			value := strings.ToLower(trimRightCRLF(paramValue[0]))
			if lVar, err = cur.NewVariable(0, oracle.StringVarType, uint(len(value))); err != nil {
				return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
			}
			lVar.SetValue(0, value)

			params[paramName] = lVar

			// stmExecDeclarePart
			stmShowDeclarePart.WriteString(fmt.Sprintf("  l_%s %s;\n", paramName, paramTypeName))
			//stmExecSetPart,
			stmShowSetPart.WriteString(fmt.Sprintf("  l_%s := %s;\n", paramName, value))
			// Вызов процедуры - Формирование строки с параметрами для вызова процедуры
			if stmExecProcParams.Len() != 0 {
				stmExecProcParams.WriteString(", ")
			}
			stmExecProcParams.WriteString(fmt.Sprintf("%s => :%s = 'true'", paramName, paramName))
			// Отображение вызова процедуры - Формирование строки с параметрами для вызова процедуры
			if stmShowProcParams.Len() != 0 {
				stmShowProcParams.WriteString(", ")
			}
			stmShowProcParams.WriteString(fmt.Sprintf("%s => l_%s", paramName, paramName))

			// Добавление вызова сохранения параметра
			if paramStoreProc != "" {
				stmExecStoreInContext.WriteString(fmt.Sprintf("  %s('%s', '%s');\n", paramStoreProc, strings.ToUpper(paramName), value))
				stmShowStoreInContext.WriteString(fmt.Sprintf("  %s('%s', '%s');\n", paramStoreProc, strings.ToUpper(paramName), value))
			}
			return nil
		}
	case oStringTab, oNumberTab, oIntegerTab, oDateTab, oBooleanTab:
		{
			value := make([]interface{}, len(paramValue))
			valueMaxLen := 0
			for i, val := range paramValue {
				val1 := val
				switch paramType {
				case oStringTab:
					{
						val1 = removeCR(val1)
					}
				case oNumberTab:
					{
						val1 = removeCR(trimRightCRLF(val1))
						if strings.HasPrefix(val1, ",") {
							val1 = "0" + val1
						}
					}
				default:
					{
						val1 = removeCR(trimRightCRLF(val1))
					}
				}
				//val := removeCR(trimRightCRLF(val))
				value[i] = val1
				if len(val1) > valueMaxLen {
					valueMaxLen = len(val1)
				}
			}
			switch paramType {
			case oStringTab:
				{
					if lVar, err = cur.NewArrayVar(oracle.StringVarType, value, uint(valueMaxLen)); err != nil {
						return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
					}
					params[paramName] = lVar

					// stmExecDeclarePart
					stmShowDeclarePart.WriteString(fmt.Sprintf("  l_%s %s;\n", paramName, paramTypeName))
					//stmExecSetPart,
					for i := range paramValue {
						stmShowSetPart.WriteString(fmt.Sprintf("  l_%s(%d) := '%s';\n", paramName, i+1, strings.Replace(trimRightCRLF(paramValue[i]), "'", "''", -1)))
					}
					// Вызов процедуры - Формирование строки с параметрами для вызова процедуры
					if stmExecProcParams.Len() != 0 {
						stmExecProcParams.WriteString(", ")
					}
					stmExecProcParams.WriteString(fmt.Sprintf("%s => :%s", paramName, paramName))
					// Отображение вызова процедуры - Формирование строки с параметрами для вызова процедуры
					if stmShowProcParams.Len() != 0 {
						stmShowProcParams.WriteString(", ")
					}
					stmShowProcParams.WriteString(fmt.Sprintf("%s => l_%s", paramName, paramName))

					// Добавление вызова сохранения параметра
					if paramStoreProc != "" {
						if lVar, err = cur.NewArrayVar(oracle.StringVarType, value, uint(valueMaxLen)); err != nil {
							return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
						}
						params[paramName+"#"] = lVar

						for i := range paramValue {
							stmExecStoreInContext.WriteString(fmt.Sprintf("  %s('%s', :%s#(%d));\n", paramStoreProc, strings.ToUpper(paramName), paramName, i+1))
							stmShowStoreInContext.WriteString(fmt.Sprintf("  %s('%s', l_%s(%d));\n", paramStoreProc, strings.ToUpper(paramName), paramName, i+1))
						}
					}

				}
			case oNumberTab:
				{
					if lVar, err = cur.NewArrayVar(oracle.FloatVarType, value, 0); err != nil {
						return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
					}
					params[paramName] = lVar
					// stmExecDeclarePart
					stmShowDeclarePart.WriteString(fmt.Sprintf("  l_%s %s;\n", paramName, paramTypeName))
					//stmExecSetPart,
					for i := range paramValue {
						stmShowSetPart.WriteString(fmt.Sprintf("  l_%s(%d) := %s;\n", paramName, i+1, trimRightCRLF(paramValue[i])))
					}
					// Вызов процедуры - Формирование строки с параметрами для вызова процедуры
					if stmExecProcParams.Len() != 0 {
						stmExecProcParams.WriteString(", ")
					}
					stmExecProcParams.WriteString(fmt.Sprintf("%s => :%s", paramName, paramName))
					// Отображение вызова процедуры - Формирование строки с параметрами для вызова процедуры
					if stmShowProcParams.Len() != 0 {
						stmShowProcParams.WriteString(", ")
					}
					stmShowProcParams.WriteString(fmt.Sprintf("%s => l_%s", paramName, paramName))

					// Добавление вызова сохранения параметра
					if paramStoreProc != "" {
						if lVar, err = cur.NewArrayVar(oracle.FloatVarType, value, uint(valueMaxLen)); err != nil {
							return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
						}
						params[paramName+"#"] = lVar

						for i := range paramValue {
							stmExecStoreInContext.WriteString(fmt.Sprintf("  %s('%s', :%s#(%d));\n", paramStoreProc, strings.ToUpper(paramName), paramName, i+1))
							stmShowStoreInContext.WriteString(fmt.Sprintf("  %s('%s', l_%s(%d));\n", paramStoreProc, strings.ToUpper(paramName), paramName, i+1))
						}
					}
				}
			case oIntegerTab:
				{
					if lVar, err = cur.NewArrayVar(oracle.Int32VarType, value, 0); err != nil {
						return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
					}
					params[paramName] = lVar
					// stmExecDeclarePart
					stmShowDeclarePart.WriteString(fmt.Sprintf("  l_%s %s;\n", paramName, paramTypeName))
					//stmExecSetPart,
					for i := range paramValue {
						stmShowSetPart.WriteString(fmt.Sprintf("  l_%s(%d) := %s;\n", paramName, i+1, trimRightCRLF(paramValue[i])))
					}
					// Вызов процедуры - Формирование строки с параметрами для вызова процедуры
					if stmExecProcParams.Len() != 0 {
						stmExecProcParams.WriteString(", ")
					}
					stmExecProcParams.WriteString(fmt.Sprintf("%s => :%s", paramName, paramName))
					// Отображение вызова процедуры - Формирование строки с параметрами для вызова процедуры
					if stmShowProcParams.Len() != 0 {
						stmShowProcParams.WriteString(", ")
					}
					stmShowProcParams.WriteString(fmt.Sprintf("%s => l_%s", paramName, paramName))
					// Добавление вызова сохранения параметра
					if paramStoreProc != "" {
						if lVar, err = cur.NewArrayVar(oracle.Int32VarType, (value), 0); err != nil {
							return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
						}
						params[paramName+"#"] = lVar
						for i := range paramValue {
							stmExecStoreInContext.WriteString(fmt.Sprintf("  %s('%s', :%s#(%d));\n", paramStoreProc, strings.ToUpper(paramName), paramName, i+1))
							stmShowStoreInContext.WriteString(fmt.Sprintf("  %s('%s', l_%s(%d));\n", paramStoreProc, strings.ToUpper(paramName), paramName, i+1))
						}
					}
				}
			case oDateTab:
				{
					valueTime := make([]interface{}, len(paramValue))
					for i, val := range paramValue {
						valueTime[i], _ = time.Parse(time.RFC3339, trimRightCRLF(val))

					}
					if lVar, err = cur.NewArrayVar(oracle.DateTimeVarType, valueTime, 0); err != nil {
						return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
					}

					params[paramName] = lVar
					// stmExecDeclarePart
					stmShowDeclarePart.WriteString(fmt.Sprintf("  l_%s %s;\n", paramName, paramTypeName))
					//stmExecSetPart,
					for i := range paramValue {
						stmShowSetPart.WriteString(fmt.Sprintf("  l_%s(%d) := to_date('%s');\n", paramName, i+1, trimRightCRLF(paramValue[i])))
					}
					// Вызов процедуры - Формирование строки с параметрами для вызова процедуры
					if stmExecProcParams.Len() != 0 {
						stmExecProcParams.WriteString(", ")
					}
					stmExecProcParams.WriteString(fmt.Sprintf("%s => :%s", paramName, paramName))
					// Отображение вызова процедуры - Формирование строки с параметрами для вызова процедуры
					if stmShowProcParams.Len() != 0 {
						stmShowProcParams.WriteString(", ")
					}
					stmShowProcParams.WriteString(fmt.Sprintf("%s => l_%s", paramName, paramName))
					// Добавление вызова сохранения параметра
					if paramStoreProc != "" {
						if lVar, err = cur.NewArrayVar(oracle.DateTimeVarType, (value), 0); err != nil {
							return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
						}
						params[paramName+"#"] = lVar
						for i := range paramValue {
							stmExecStoreInContext.WriteString(fmt.Sprintf("  %s('%s', :%s#(%d));\n", paramStoreProc, strings.ToUpper(paramName), paramName, i+1))
							stmShowStoreInContext.WriteString(fmt.Sprintf("  %s('%s', l_%s(%d));\n", paramStoreProc, strings.ToUpper(paramName), paramName, i+1))
						}
					}
				}
			default:
				{
					return errgo.Newf("error creating variable for %s(%T): Invalid subtype %v", paramName, value, paramType)
				}
			}
			return nil
		}
	default:
		{
			//Параметры, отсутствующие в списке параметров процедуры.
			//Сохраняются только в контексте
			value := trimRightCRLF(paramValue[0])
			// stmExecDeclarePart
			stmShowDeclarePart.WriteString(fmt.Sprintf("  l_%s varchar2(32767);\n", paramName))
			//stmExecSetPart,
			stmShowSetPart.WriteString(fmt.Sprintf("  l_%s := '%s';\n", paramName, strings.Replace(value, "'", "''", -1)))

			// Добавление вызова сохранения параметра
			if paramStoreProc != "" {
				if lVar, err = cur.NewVariable(0, oracle.StringVarType, uint(len(value))); err != nil {
					return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
				}
				lVar.SetValue(0, value)

				params[paramName+"#"] = lVar
				stmExecStoreInContext.WriteString(fmt.Sprintf("  %s('%s', :%s#);\n", paramStoreProc, strings.ToUpper(paramName), paramName))
				stmShowStoreInContext.WriteString(fmt.Sprintf("  %s('%s', l_%s);\n", paramStoreProc, strings.ToUpper(paramName), paramName))
			}
			return nil
		}
	}
}

func UnMask(err error) *oracle.Error {
	oraErr, ok := err.(*oracle.Error)
	if ok {
		return oraErr
	}
	if errg, ok := err.(*errgo.Err); ok {
		return UnMask(errg.Underlying())
	}
	return nil
}

func ExtractFileName(contentDisposition string) string {
	r := ""
	for _, v := range strings.Split(contentDisposition, "; ") {
		if strings.HasPrefix(v, "filename=") {
			_, r = filepath.Split(strings.Replace(strings.Replace(v, "filename=\"", "", -1), "\"", "", -1))
			return r
		}
	}
	return r
}

var (
	crlf = string([]byte{13, 10})
	cr   = string([]byte{13})
	lf   = string([]byte{10})
)

func trimRightCRLF(val string) string { return strings.TrimRight(val, crlf) }

func removeCR(val string) string { return strings.Replace(val, cr, "", -1) }

func addCR(val string) string {

	var out []byte
	var prevRune rune

	for i := 0; i < len(val); {
		r, wid := utf8.DecodeRuneInString(val[i:])
		if wid == 1 {
			if r == rune(10) {
				if prevRune != rune(13) {
					out = append(out, cr...)
				}
			}
		}
		prevRune = r
		out = append(out, val[i:i+wid]...)
		i += wid
	}
	return string(out)
}

func prepareStringParam(
	cur *oracle.Cursor, params map[string]interface{},
	paramName string, paramValue []string,
	paramTypeName string,
	paramStoreProc string,
	stmExecDeclarePart, stmShowDeclarePart,
	stmExecSetPart, stmShowSetPart,
	stmExecProcParams, stmShowProcParams,
	stmExecStoreInContext, stmShowStoreInContext *bytes.Buffer,
	/*
		paramsForCall, paramsForStore bytes.Buffer*/) error {
	var (
		lVar *oracle.Variable
		err  error
	)

	value := removeCR(paramValue[0])

	if lVar, err = cur.NewVariable(0, oracle.StringVarType, uint(len(value))); err != nil {
		return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
	}
	lVar.SetValue(0, value)

	params[paramName] = lVar

	// stmExecDeclarePart
	iLen := len(value)
	if iLen == 0 {
		// Для того, чтобы избежать ситуации VARCHAR2(0);
		iLen = iLen + 1
	}
	stmShowDeclarePart.WriteString(fmt.Sprintf("  l_%s %s(%d);\n", paramName, paramTypeName, iLen))
	//stmExecSetPart,
	stmShowSetPart.WriteString(fmt.Sprintf("  l_%s := '%s';\n", paramName, strings.Replace(value, "'", "''", -1)))
	// Вызов процедуры - Формирование строки с параметрами для вызова процедуры
	if stmExecProcParams.Len() != 0 {
		stmExecProcParams.WriteString(", ")
	}
	stmExecProcParams.WriteString(fmt.Sprintf("%s => :%s", paramName, paramName))

	// Отображение вызова процедуры - Формирование строки с параметрами для вызова процедуры
	if stmShowProcParams.Len() != 0 {
		stmShowProcParams.WriteString(", ")
	}
	stmShowProcParams.WriteString(fmt.Sprintf("%s => l_%s", paramName, paramName))

	// Добавление вызова сохранения параметра
	if paramStoreProc != "" {
		if lVar, err = cur.NewVar(&value); err != nil {
			return errgo.Newf("error creating variable for %s(%T): %s", paramName, value, err)
		}
		params[paramName+"#"] = lVar
		stmExecStoreInContext.WriteString(fmt.Sprintf("  %s('%s', :%s#);\n", paramStoreProc, strings.ToUpper(paramName), paramName))
		stmShowStoreInContext.WriteString(fmt.Sprintf("  %s('%s', l_%s);\n", paramStoreProc, strings.ToUpper(paramName), paramName))
	}
	return nil
}
