// worker_test
package otasker

import (
	//	"fmt"
	//	"net/http"
	//	"net/http/httptest"
	//	"net/http/pprof"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vsdutka/mltpart"
)

type test struct {
	name         string
	path         string
	sessionID    string
	taskID       string
	userName     string
	userPass     string
	connStr      string
	procName     string
	procCreate   string
	procDrop     string
	urlValues    url.Values
	files        *mltpart.Form
	waitTimeout  time.Duration
	idleTimeout  time.Duration
	afterTimeout time.Duration
	resCode      int
	resContent   string
}

func workerRun(t *testing.T, v test) {

	if v.procCreate != "" {
		err := exec(*dsn, v.procCreate)
		if err != nil {
			t.Fatalf("%s - Error when run \"%s\": %s", v.name, v.procCreate, err.Error())
		}
	}
	defer func() {
		time.Sleep(v.afterTimeout)
		if v.procDrop != "" {
			err := exec(*dsn, v.procDrop)
			if err != nil {
				t.Fatalf("%s - Error when run \"%s\": %s", v.name, v.procDrop, err.Error())
			}
		}
	}()

	res := Run(
		v.path,
		ClassicTasker,
		v.sessionID,
		v.taskID,
		v.userName,
		v.userPass,
		v.connStr,
		"WEX.WS",
		stm_init,
		"",
		"WWV_DOCUMENT",
		cgi,
		v.procName,
		v.urlValues,
		v.files,
		v.waitTimeout,
		v.idleTimeout,
		".\\log.log")
	if res.StatusCode != v.resCode {
		t.Log(string(res.Content))
		t.Fatalf("%s: StatusCode - got %v,\nwant %v", v.name, res.StatusCode, v.resCode)
	}
	if string(res.Content) != v.resContent {
		t.Fatalf("%s: Content - got \"%v\",\nwant \"%v\"", v.name, string(res.Content), v.resContent)
	}

}
func TestWorkerRun(t *testing.T) {
	var vpath = strings.ToUpper("TestWorkerRun")
	var tests = []test{
		{
			name:      "Вызов простой процедуры",
			path:      vpath,
			sessionID: "sess1",
			taskID:    "TASK1",
			userName:  dsn_user,
			userPass:  dsn_passw,
			connStr:   dsn_sid,
			procName:  "TestWorkerRun",
			procCreate: `
create or replace procedure TestWorkerRun(ap in varchar2) is 
begin
  htp.set_ContentType('text/plain');
  htp.add_CustomHeader('CUSTOM_HEADER: HEADER
CUSTOM_HEADER1: HEADER1
');
  htp.prn(ap);
  hrslt.ADD_FOOTER := false;
  rollback;
end;`,
			procDrop:  `drop procedure TestWorkerRun`,
			urlValues: url.Values{"ap": []string{"1"}},
			files: &mltpart.Form{
				Value: map[string][]string{},
				File:  map[string][]*mltpart.FileHeader{},
			},
			waitTimeout:  10 * time.Second,
			idleTimeout:  2 * time.Second,
			afterTimeout: 3 * time.Second,
			resCode:      200,
			resContent:   "1",
		},
		{
			name:      "Неправильное имя пользователя или пароль",
			path:      vpath,
			sessionID: "sess2",
			taskID:    "TASK2",
			userName:  "user",
			userPass:  dsn_passw,
			connStr:   dsn_sid,
			procName:  "TestWorkerRun",
			procCreate: `
		create or replace procedure TestWorkerRun(ap in varchar2) is
		begin
		  htp.set_ContentType('text/plain');
		  htp.add_CustomHeader('CUSTOM_HEADER: HEADER
		CUSTOM_HEADER1: HEADER1
		');
		  htp.prn(ap);
		  hrslt.ADD_FOOTER := false;
		  rollback;
		end;`,
			procDrop:  `drop procedure TestWorkerRun`,
			urlValues: url.Values{"ap": []string{"1"}},
			files: &mltpart.Form{
				Value: map[string][]string{},
				File:  map[string][]*mltpart.FileHeader{},
			},
			waitTimeout:  10 * time.Second,
			idleTimeout:  1 * time.Second,
			afterTimeout: 2 * time.Second,
			resCode:      StatusInvalidUsernameOrPassword,
			resContent:   "",
		},
		{
			name:       "Пользователь заблокирован",
			path:       vpath,
			sessionID:  "sess3",
			taskID:     "TASK3",
			userName:   "TEST001",
			userPass:   "1",
			connStr:    dsn_sid,
			procName:   "a.root$.startup",
			procCreate: "begin execute immediate 'create user \"TEST001\" identified by \"1\" account lock'; execute immediate 'grant connect to TEST001'; end;",
			procDrop:   "drop user \"TEST001\"",
			urlValues:  url.Values{},
			files: &mltpart.Form{
				Value: map[string][]string{},
				File:  map[string][]*mltpart.FileHeader{},
			},
			waitTimeout:  10 * time.Second,
			idleTimeout:  1 * time.Second,
			afterTimeout: 2 * time.Second,
			resCode:      StatusAccountIsLocked,
			resContent:   "",
		},
		{
			name:      "Длинный запрос 1 - червяк",
			path:      vpath,
			sessionID: "sess4",
			taskID:    "TASK4",
			userName:  dsn_user,
			userPass:  dsn_passw,
			connStr:   dsn_sid,
			procName:  "TestWorkerRun",
			procCreate: `
create or replace procedure TestWorkerRun(ap in varchar2) is 
begin
  htp.set_ContentType('text/plain');
  htp.add_CustomHeader('CUSTOM_HEADER: HEADER
CUSTOM_HEADER1: HEADER1
');
  htp.prn(ap);
  hrslt.ADD_FOOTER := false;
  dbms_lock.sleep(15);
  rollback;
end;`,
			procDrop:  ``,
			urlValues: url.Values{"ap": []string{"1"}},
			files: &mltpart.Form{
				Value: map[string][]string{},
				File:  map[string][]*mltpart.FileHeader{},
			},
			waitTimeout:  10 * time.Second,
			idleTimeout:  1 * time.Second,
			afterTimeout: 0 * time.Second,
			resCode:      StatusWaitPage,
			resContent:   "",
		},
		{
			name:       "Длинный запрос 1 - результат",
			path:       vpath,
			sessionID:  "sess4",
			taskID:     "TASK4",
			userName:   dsn_user,
			userPass:   dsn_passw,
			connStr:    dsn_sid,
			procName:   "TestWorkerRun",
			procCreate: ``,
			procDrop:   ``,
			urlValues:  url.Values{"ap": []string{"1"}},
			files: &mltpart.Form{
				Value: map[string][]string{},
				File:  map[string][]*mltpart.FileHeader{},
			},
			waitTimeout:  10 * time.Second,
			idleTimeout:  1 * time.Second,
			afterTimeout: 0 * time.Second,
			resCode:      200,
			resContent:   "1",
		},
		{
			name:       "Длинный запрос 2 - червяк",
			path:       vpath,
			sessionID:  "sess5",
			taskID:     "TASK5.1",
			userName:   dsn_user,
			userPass:   dsn_passw,
			connStr:    dsn_sid,
			procName:   "TestWorkerRun",
			procCreate: ``,
			procDrop:   ``,
			urlValues:  url.Values{"ap": []string{"1"}},
			files: &mltpart.Form{
				Value: map[string][]string{},
				File:  map[string][]*mltpart.FileHeader{},
			},
			waitTimeout:  10 * time.Second,
			idleTimeout:  1 * time.Second,
			afterTimeout: 0 * time.Second,
			resCode:      StatusWaitPage,
			resContent:   "",
		},
		{
			name:       "Длинный запрос 2 - прервать запрос",
			path:       vpath,
			sessionID:  "sess5",
			taskID:     "TASK5.2",
			userName:   dsn_user,
			userPass:   dsn_passw,
			connStr:    dsn_sid,
			procName:   "TestWorkerRun",
			procCreate: ``,
			procDrop:   ``,
			urlValues:  url.Values{"ap": []string{"1"}},
			files: &mltpart.Form{
				Value: map[string][]string{},
				File:  map[string][]*mltpart.FileHeader{},
			},
			waitTimeout:  1 * time.Second,
			idleTimeout:  1 * time.Second,
			afterTimeout: 0 * time.Second,
			resCode:      StatusBreakPage,
			resContent:   "",
		},
		{
			name:       "Длинный запрос 2 - результат",
			path:       vpath,
			sessionID:  "sess5",
			taskID:     "TASK5.1",
			userName:   dsn_user,
			userPass:   dsn_passw,
			connStr:    dsn_sid,
			procName:   "TestWorkerRun",
			procCreate: ``,
			procDrop:   `drop procedure TestWorkerRun`,
			urlValues:  url.Values{"ap": []string{"1"}},
			files: &mltpart.Form{
				Value: map[string][]string{},
				File:  map[string][]*mltpart.FileHeader{},
			},
			waitTimeout:  10 * time.Second,
			idleTimeout:  1 * time.Second,
			afterTimeout: 3 * time.Second,
			resCode:      200,
			resContent:   "1",
		},
	}

	for _, v := range tests {
		workerRun(t, v)
	}

	wlock.Lock()
	if len(wlist[vpath]) > 0 {
		for k, _ := range wlist[vpath] {
			t.Log(k)
		}
		t.Fatalf("len(wlist[vpath]) = %d", len(wlist[vpath]))
	}
	wlock.Unlock()
}

func TestWorkerBreak(t *testing.T) {
	var vpath = strings.ToUpper("TestWorkerBreak")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		workerRun(t, test{
			name:      "Длинный запрос 3 - запрос прерван",
			path:      vpath,
			sessionID: "sess6",
			taskID:    "TASK6",
			userName:  dsn_user,
			userPass:  dsn_passw,
			connStr:   dsn_sid,
			procName:  "TestWorkerRun",
			procCreate: `
create or replace procedure TestWorkerRun(ap in varchar2) is 
begin
  htp.set_ContentType('text/plain');
  htp.add_CustomHeader('CUSTOM_HEADER: HEADER
CUSTOM_HEADER1: HEADER1
');
  htp.prn(ap);
  hrslt.ADD_FOOTER := false;
  dbms_lock.sleep(15);
  rollback;
end;`,
			procDrop:  ``,
			urlValues: url.Values{"ap": []string{"1"}},
			files: &mltpart.Form{
				Value: map[string][]string{},
				File:  map[string][]*mltpart.FileHeader{},
			},
			waitTimeout:  10 * time.Second,
			idleTimeout:  1 * time.Second,
			afterTimeout: 0 * time.Second,
			resCode:      StatusRequestWasInterrupted,
			resContent:   "",
		})
		wg.Done()
	}()

	go func() {
		//Пробуем прервать сессию, которой нет. Ошибки не должно быть
		err := Break(vpath, "sess6-1")
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Second)
		err = Break(vpath, "sess6")
		if err != nil {
			t.Fatal(err)
		}
		wg.Done()
	}()
	wg.Wait()
}

//func TestWorkerTimerUsage(t *testing.T) {
//	var vpath = strings.ToUpper("TestWorkerTimerUsage")
//	var wg sync.WaitGroup
//	wg.Add(2)
//	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		if strings.HasPrefix(r.URL.Path, "/debug/pprof/") {
//			pprof.Index(w, r)
//			return
//		}
//		switch r.URL.Path {
//		case "/q":
//			wg.Done()
//		case "/debug/pprof/":
//			pprof.Index(w, r)
//		case "/debug/pprof/cmdline":
//			pprof.Cmdline(w, r)
//		case "/debug/pprof/profile":
//			pprof.Profile(w, r)
//		case "/debug/pprof/symbol":
//			pprof.Symbol(w, r)
//		case "/debug/pprof/trace":
//			pprof.Trace(w, r)
//		}
//		fmt.Println(r.URL.Path)
//	}))
//	defer ts.Close()
//	fmt.Println(ts.URL)
//	var tests = []test{
//		{
//			name:       "Длинный запрос 1 - червяк",
//			path:       vpath,
//			sessionID:  "sess4",
//			taskID:     "TASK4",
//			userName:   dsn_user,
//			userPass:   dsn_passw,
//			connStr:    dsn_sid,
//			procName:   "TestWorkerRun",
//			procCreate: createLongProc,
//			procDrop:   ``,
//			urlValues:  url.Values{"ap": []string{"1"}},
//			files: &mltpart.Form{
//				Value: map[string][]string{},
//				File:  map[string][]*mltpart.FileHeader{},
//			},
//			waitTimeout:  10 * time.Second,
//			idleTimeout:  1 * time.Second,
//			afterTimeout: 0 * time.Second,
//			resCode:      StatusWaitPage,
//			resContent:   "",
//		},
//	}

//	for _, v := range tests {
//		workerRun(t, v)
//	}
//	wg.Wait()
//	fmt.Println("after wait")
//	wlock.Lock()
//	if len(wlist[vpath]) > 0 {
//		for k, _ := range wlist[vpath] {
//			t.Log(k)
//		}
//		t.Fatalf("len(wlist[vpath]) = %d", len(wlist[vpath]))
//	}
//	wlock.Unlock()

//}

//const (
//	createLongProc = `
//create or replace procedure TestWorkerRun(ap in varchar2) is
//begin
//  htp.set_ContentType('text/plain');
//  htp.add_CustomHeader('CUSTOM_HEADER: HEADER
//CUSTOM_HEADER1: HEADER1
//');
//  htp.prn(ap);
//  hrslt.ADD_FOOTER := false;
//  dbms_lock.sleep(15);
//  rollback;
//end;`
//)
