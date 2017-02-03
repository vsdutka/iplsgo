// otasker_test
package otasker

import (
	"fmt"
	"net/textproto"
	"net/url"
	"strings"
	"testing"
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/goracle.v1/oracle"
)

func exec(dsn, stm string) error {
	if !(dsn != "") {
		return errgo.New("cannot test connection without dsn!")
	}
	user, passw, sid := oracle.SplitDSN(dsn)
	var err error
	conn, err := oracle.NewConnection(user, passw, sid, false)
	if err != nil {
		return errgo.New("cannot create connection: " + err.Error())
	}
	if err = conn.Connect(0, false); err != nil {
		return errgo.New("error connecting: " + err.Error())
	}
	defer conn.Close()
	cur := conn.NewCursor()
	defer cur.Close()
	return cur.Execute(stm, nil, nil)
}

const (
	stm_drop_p = `drop procedure %s`
	stm_init   = `declare 
  f integer;
  r integer;
begin
  f:=dbms_sql.open_cursor;
  dbms_sql.parse(f,'ALTER SESSION SET NLS_DATE_FORMAT='||CHR(39)||'DD/MM/RRRR HH24:MI'||CHR(39),1);
  r:=dbms_sql.execute(f);
  dbms_sql.close_cursor(f);
  f:=dbms_sql.open_cursor;
  dbms_sql.parse(f,'ALTER SESSION SET NLS_NUMERIC_CHARACTERS='||CHR(39)||', '||CHR(39),1);
  r:=dbms_sql.execute(f);
  dbms_sql.close_cursor(f);
end;`
)

var cgi = map[string]string{"SERVER_SOFTWARE": "iPLSQL",
	"SERVER_NAME":       "localhost",
	"GATEWAY_INTERFACE": "CGI/1.1",
	"PT_DC_ID":          "-5",
}

func TestTaskerDoubleClose(t *testing.T) {
	tasker := NewOwaClassicProcRunner()()
	defer func() {
		err := tasker.CloseAndFree()
		if err != nil {
			t.Fatalf("Error when tasker.CloseAndFree(): %s", err.Error())
		}
		err = tasker.CloseAndFree()
		if err != nil {
			t.Fatalf("Error when tasker.CloseAndFree(): %s", err.Error())
		}
	}()
}

func TestTaskerConnect(t *testing.T) {
	tasker := NewOwaClassicProcRunner()()
	defer func() {
		err := tasker.CloseAndFree()
		if err != nil {
			t.Fatalf("Error when tasker.CloseAndFree(): %s", err.Error())
		}
	}()
	var (
		urlParams = make(url.Values)
	)
	r := tasker.Run("sessionID", "taskID", "user", "password", dsn_sid, "", "", "", "", cgi, "test_p1", urlParams, nil, ".\\log.log")
	if r.StatusCode != StatusInvalidUsernameOrPassword {
		t.Fatalf("StatusCode - got %v,\nwant %v", r.StatusCode, StatusInvalidUsernameOrPassword)
	}
}

func TestTaskerReconnect(t *testing.T) {
	tasker := NewOwaClassicProcRunner()()
	defer func() {
		err := tasker.CloseAndFree()
		if err != nil {
			t.Fatalf("Error when tasker.CloseAndFree(): %s", err.Error())
		}
	}()
	var (
		urlParams = make(url.Values)
	)
	r := tasker.Run("sessionID", "taskID", dsn_user, dsn_passw, dsn_sid, "", "", "", "", cgi, "root$.startup", urlParams, nil, ".\\log.log")
	if r.StatusCode != 200 {
		t.Fatalf("StatusCode - got %v,\nwant %v", r.StatusCode, 200)
	}
	r = tasker.Run("sessionID", "taskID", strings.ToUpper(dsn_user), dsn_passw, dsn_sid, "", "", "", "", cgi, "root$.startup", urlParams, nil, ".\\log.log")
	if r.StatusCode != 200 {
		t.Fatalf("StatusCode - got %v,\nwant %v", r.StatusCode, 200)
	}
}

func run_p(t *testing.T, procName, paramType string, paramValue []string, reqFiles *Form, result string) {
	const (
		stm_create_pkg = `
create or replace package pkg_types is
  type tinteger_table is table of integer index by binary_integer;
  type tdate_table is table of date index by binary_integer;
end;`
		stm_drop_pkg = `drop package pkg_types`

		stm_create_p = `
create or replace procedure %s(ap in %s) is 
begin
  htp.set_ContentType('text/plain');
  htp.add_CustomHeader('CUSTOM_HEADER: HEADER
CUSTOM_HEADER1: HEADER1
');
  %s
  hrslt.ADD_FOOTER := false;
  rollback;
end;`
	)

	s := "htp.prn(ap);"
	if (len(paramValue) > 1) || ((reqFiles != nil) && (reqFiles.File["ap"] != nil) && (len(reqFiles.File["ap"]) > 1)) {
		s = "for i in ap.first()..ap.last() loop htp.prn(ap(i)); end loop;"
	}

	err := exec(*dsn, stm_create_pkg)
	if err != nil {
		t.Fatalf("%s - Error when create package for types: %s", paramType, err.Error())
	}

	err = exec(*dsn, fmt.Sprintf(stm_create_p, procName, paramType, s))
	if err != nil {
		t.Fatalf("%s - Error when create procedure \"%s\": %s", paramType, procName, err.Error())
	}

	tasker := NewOwaClassicProcRunner()()
	defer func() {
		err := tasker.CloseAndFree()
		if err != nil {
			t.Fatalf("%s - Error when tasker.CloseAndFree(): %s", paramType, err.Error())
		}
	}()
	var (
		urlParams = make(url.Values)
	)

	for _, v := range paramValue {
		urlParams.Add("ap", v)
	}

	r := tasker.Run("sessionID", procName, dsn_user, dsn_passw, dsn_sid, "",
		stm_init, "session_final.final;", "wwv_document", cgi,
		procName, urlParams, reqFiles, ".\\log.log")
	if r.StatusCode != 200 {
		t.Log(string(r.Content))
		t.Fatalf("%s - StatusCode - got %v,\nwant %v", paramType, r.StatusCode, 200)
	}
	if string(r.Content) != result {
		t.Fatalf("%s - Content - got %v,\nwant %v", paramType, string(r.Content), result)
	}
	if r.ContentType != "text/plain" {
		t.Fatalf("%s - ContentType - got %v,\nwant %v", paramType, r.ContentType, "text/plain")
	}
	if r.Headers != "CUSTOM_HEADER: HEADER\nCUSTOM_HEADER1: HEADER1\n" {
		t.Fatalf("%s - Headers - got %v,\nwant %v", paramType, r.Headers, "CUSTOM_HEADER: HEADER\nCUSTOM_HEADER1: HEADER1\n")
	}

	err = exec(*dsn, fmt.Sprintf(stm_drop_p, procName))
	if err != nil {
		t.Fatalf("%s - Error when drop procedure \"%s\": %s", paramType, procName, err.Error())
	}
	err = exec(*dsn, stm_drop_pkg)
	if err != nil {
		t.Fatalf("%s - Error when drop package for types: %s", paramType, err.Error())
	}
}

func TestTaskerRun(t *testing.T) {
	var (
		data = []struct {
			procName    string
			paramType   string
			paramValues []string
			reqFiles    *Form
			result      string
		}{
			{
				procName:    "test_run_p1",
				paramType:   "varchar2",
				paramValues: []string{"test"},
				reqFiles: &Form{
					Value: map[string][]string{},
					File:  map[string][]*FileHeader{},
				},
				result: "test",
			},
			{
				procName:    "test_run_p2",
				paramType:   "number",
				paramValues: []string{"123.5"},
				reqFiles: &Form{
					Value: map[string][]string{},
					File:  map[string][]*FileHeader{},
				},
				result: "123,5",
			},
			{
				procName:    "test_run_p2",
				paramType:   "number",
				paramValues: []string{"123,5"},
				reqFiles: &Form{
					Value: map[string][]string{},
					File:  map[string][]*FileHeader{},
				},
				result: "1235",
			},
			{
				procName:    "test_run_p2",
				paramType:   "number",
				paramValues: []string{""},
				reqFiles: &Form{
					Value: map[string][]string{},
					File:  map[string][]*FileHeader{},
				},
				result: "",
			},
			{
				procName:    "test_run_p2",
				paramType:   "number",
				paramValues: []string{"-1"},
				reqFiles: &Form{
					Value: map[string][]string{},
					File:  map[string][]*FileHeader{},
				},
				result: "-1",
			},
			{
				procName:    "test_run_p3",
				paramType:   "integer",
				paramValues: []string{"5"},
				reqFiles: &Form{
					Value: map[string][]string{},
					File:  map[string][]*FileHeader{},
				},
				result: "5",
			},
			{
				procName:    "test_run_p3",
				paramType:   "integer",
				paramValues: []string{"-15"},
				reqFiles: &Form{
					Value: map[string][]string{},
					File:  map[string][]*FileHeader{},
				},
				result: "-15",
			},
			{
				procName:    "test_run_p4",
				paramType:   "date",
				paramValues: []string{"21/12/2015 09:00"},
				reqFiles:    nil,
				result:      "21/12/2015 09:00",
			},
			{
				procName:    "test_run_p5",
				paramType:   "owa.vc_arr",
				paramValues: []string{"s1", "s2", "s3"},
				reqFiles: &Form{
					Value: map[string][]string{},
					File:  map[string][]*FileHeader{},
				},
				result: "s1s2s3",
			},
			{
				procName:    "test_run_p6",
				paramType:   "sys.dbms_describe.number_table",
				paramValues: []string{"100.456", "534534534.234200"},
				reqFiles: &Form{
					Value: map[string][]string{},
					File:  map[string][]*FileHeader{},
				},
				result: "100,456534534534,2342",
			},
			{
				procName:    "test_run_p7",
				paramType:   "pkg_types.tinteger_table",
				paramValues: []string{"100", "500"},
				reqFiles: &Form{
					Value: map[string][]string{},
					File:  map[string][]*FileHeader{},
				},
				result: "100500",
			},
			{
				procName:  "test_run_p6",
				paramType: "pkg_types.tdate_table",
				paramValues: []string{time.Date(2015, 12, 21, 9, 50, 0, 0, time.Local).Format(time.RFC3339),
					time.Date(2017, 12, 21, 9, 0, 0, 0, time.Local).Format(time.RFC3339),
					time.Date(2015, 12, 01, 9, 0, 0, 0, time.Local).Format(time.RFC3339)},
				reqFiles: &Form{
					Value: map[string][]string{},
					File:  map[string][]*FileHeader{},
				},
				result: "21/12/2015 09:5021/12/2017 09:0001/12/2015 09:00",
			},
			{
				procName:    "test_run_file_upload",
				paramType:   "varchar2",
				paramValues: []string{},
				reqFiles: &Form{
					Value: map[string][]string{},
					File: map[string][]*FileHeader{"ap": []*FileHeader{
						&FileHeader{
							Filename: "test_file.txt1",
							Header: textproto.MIMEHeader{
								"Content-Disposition": []string{"filename=\"test_file_1.txt\""},
								"Content-Type":        []string{"text/html"},
							},
							content: []byte{1, 2, 3, 4, 5, 6, 7, 8},
							tmpfile: "",
							lastArg: "",
						},
					}},
				},
				result: "test_file_1.txt",
			},
			{
				procName:    "test_run_file_upload_2",
				paramType:   "owa.vc_arr",
				paramValues: []string{},
				reqFiles: &Form{
					Value: map[string][]string{},
					File: map[string][]*FileHeader{
						"ap": []*FileHeader{
							&FileHeader{
								Filename: "test_file.txt1",
								Header: textproto.MIMEHeader{
									"Content-Disposition": []string{"filename=\"test_file_1.txt\""},
									"Content-Type":        []string{"text/html"},
								},
								content: []byte{1, 2, 3, 4, 5, 6, 7, 8},
								tmpfile: "",
								lastArg: "",
							},
							&FileHeader{
								Filename: "test_file.txt1",
								Header: textproto.MIMEHeader{
									"Content-Disposition": []string{"filename=\"test_file_2.txt\""},
									"Content-Type":        []string{"text/html"},
								},
								content: []byte{1, 2, 3, 4, 5, 6, 7, 8},
								tmpfile: "",
								lastArg: "",
							},
						}},
				},
				result: "test_file_1.txttest_file_2.txt",
			},
			{
				procName:    "test_run_64k",
				paramType:   "owa.vc_arr",
				paramValues: []string{strings.Repeat("!", 30000), strings.Repeat("2", 30000), strings.Repeat("@", 30000)},
				reqFiles: &Form{
					Value: map[string][]string{},
					File:  map[string][]*FileHeader{},
				},
				result: strings.Repeat("!", 30000) + strings.Repeat("2", 30000) + strings.Repeat("@", 30000),
			},
		}
	)
	for k, _ := range data {
		run_p(t, data[k].procName, data[k].paramType, data[k].paramValues, data[k].reqFiles, data[k].result)
	}
}
