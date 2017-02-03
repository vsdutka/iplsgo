// describe_test
package otasker

import (
	"flag"
	"gopkg.in/goracle.v1/oracle"
	"sync"
	"testing"
	"time"
)

var (
	dsn       = flag.String("dsn", "", "Oracle DSN (user/passw@sid)")
	dsn_user  string
	dsn_passw string
	dsn_sid   string
)

func init() {
	flag.Parse()
	dsn_user, dsn_passw, dsn_sid = oracle.SplitDSN(*dsn)
}

func getConnection(t *testing.T) (conn *oracle.Connection) {
	if !(*dsn != "") {
		t.Fatalf("cannot test connection without dsn!")
	}

	var err error
	conn, err = oracle.NewConnection(dsn_user, dsn_passw, dsn_sid, false)
	if err != nil {
		t.Fatal("cannot create connection: " + err.Error())
	}
	if err = conn.Connect(0, false); err != nil {
		t.Fatal("error connecting: " + err.Error())
	}
	return conn
}

func TestDescribe(t *testing.T) {
	conn := getConnection(t)
	defer conn.Close()
	if err := Describe(conn, dsn_sid, "f"); err != nil {
		t.Log(err)
		t.Fail()
	}

	if _, _, err := ProcedureInfo(dsn_sid, "f"); err != nil {
		t.Log(err)
		t.Fail()
	}
	if err := Describe(conn, dsn_sid, "f1"); err == nil {
		t.Fatalf("procedure \"f1\" should not be exists")
	}
	if _, _, err := ProcedureInfo(dsn_sid+"!", "f1"); err == nil {
		t.Fatalf("procedure \"f1\" should not be exists")
	}
}

func createProc(t *testing.T, conn *oracle.Connection) {
	cur := conn.NewCursor()
	defer cur.Close()
	if err := cur.Execute(stm, nil, nil); err != nil {
		t.Log(err)
		t.Fail()
	}
}

func dropProc(t *testing.T, conn *oracle.Connection) {
	cur := conn.NewCursor()
	defer cur.Close()
	if err := cur.Execute("drop procedure test_descr", nil, nil); err != nil {
		t.Log(err)
		t.Fail()
	}
}

func TestDescribeAfterRecompile(t *testing.T) {
	conn := getConnection(t)
	defer conn.Close()
	createProc(t, conn)

	var (
		err        error
		timestamp1 time.Time
		timestamp2 time.Time
	)
	if err = Describe(conn, dsn_sid, "test_descr"); err != nil {
		t.Log(err)
		t.Fail()
	}
	if timestamp1, _, err = ProcedureInfo(dsn_sid, "test_descr"); err != nil {
		t.Log(err)
		t.Fail()
	}
	time.After(2 * time.Second)
	if err = Describe(conn, dsn_sid, "test_descr"); err != nil {
		t.Log(err)
		t.Fail()
	}
	if timestamp2, _, err = ProcedureInfo(dsn_sid, "test_descr"); err != nil {
		t.Log(err)
		t.Fail()
	}

	if timestamp1 != timestamp2 {
		t.Fatalf("got %v,\nwant %v", timestamp2, timestamp1)
	}
	time.After(4 * time.Second)
	dropProc(t, conn)
	createProc(t, conn)
	if err = Describe(conn, dsn_sid, "test_descr"); err != nil {
		t.Log(err)
		t.Fail()
	}
	if timestamp2, _, err = ProcedureInfo(dsn_sid, "test_descr"); err != nil {
		t.Log(err)
		t.Fail()
	}
	if timestamp1 == timestamp2 {
		t.Fatalf("got %s,\nwant %s", timestamp2.Format(time.RFC3339Nano), timestamp1.Format(time.RFC3339Nano))
	}
}

//func BenchmarkDescribe(b *testing.B) {
//	conn := getConnection(b)
//	defer conn.Close()
//	Describe(conn, dsn_sid, "root$.startup")
//	b.ResetTimer()
//	for i := 0; i < 1; i++ {
//		if err := Describe(conn, dsn_sid, "root$.startup"); err != nil {
//			b.Log(err)
//			b.Fail()
//		}
//	}
//}

func TestDescribe1(t *testing.T) {
	conn := getConnection(t)
	defer conn.Close()
	for i := 0; i < 1000; i++ {
		if err := Describe(conn, dsn_sid, "f"); err != nil {
			t.Log(err)
			t.Fail()
		}
	}
}
func TestDescribeNotExists(t *testing.T) {
	conn := getConnection(t)
	defer conn.Close()
	if err := Describe(conn, dsn_sid, "f1"); err == nil {
		t.Fatal("procedure \"f1\" should not be exists")
	}
}

func TestDescribeConcurent10(t *testing.T) {
	var wg sync.WaitGroup

	for j := 0; j < 30; j++ {
		wg.Add(1)
		go func() {
			TestDescribe1(t)
			wg.Done()
		}()
	}
	wg.Wait()
}

func TestArguments(t *testing.T) {
	conn := getConnection(t)
	defer conn.Close()
	if err := Describe(conn, dsn_sid, "test_descr"); err != nil {
		t.Log(err)
		t.Fail()
	}
	for j := 0; j < 10; j++ {
		for _, v := range []struct {
			name         string
			dataType     int32
			dataSubType  int32
			dataTypeName string
			level        int32
			length       int32
		}{
			{name: "AP1", dataType: 1, dataTypeName: "VARCHAR2"},
			{name: "AP2", dataType: 1, dataTypeName: "STRING"},
			{name: "AP3", dataType: 1, dataTypeName: "CHAR"},
			{name: "AP4", dataType: 2, dataTypeName: "NUMBER"},
			{name: "AP6", dataType: 12, dataTypeName: "SYS.DBMS_DESCRIBE.NUMBER_TABLE"},
			{name: "AP7", dataType: 2, dataTypeName: "FLOAT"},
			{name: "AP9", dataType: 5, dataTypeName: "INTEGER"},
			{name: "AP10", dataType: 5, dataTypeName: "PLS_INTEGER"},
			{name: "AP11", dataType: 4, dataTypeName: "BOOLEAN"},
			{name: "AP12", dataType: 11, dataTypeName: "PUBLIC.OWA.VC_ARR"},
			{name: "AP13", dataType: 11, dataTypeName: "PUBLIC.OWA.NC_ARR"},
			{name: "AP14", dataType: 1, dataTypeName: "VARCHAR2"},
		} {
			dataType, dataTypeName, err := ArgumentInfo(dsn_sid, "test_descr", v.name)
			if err != nil {
				t.Log(err)
				t.Fail()
			}
			if dataType != v.dataType {
				t.Fatalf("dataType - got %v,\nwant %v", v.dataType, dataType)
			}
			if dataTypeName != v.dataTypeName {
				t.Fatalf("dataTypeName - got %v,\nwant %v", v.dataTypeName, dataTypeName)
			}
		}
	}
	_, _, err := ArgumentInfo(dsn_sid, "test_descr", "q")
	if err == nil {
		t.Log("argument \"q\" for \"test_descr1\" should not be exists")
		t.Fail()
	}
	_, _, err = ArgumentInfo(dsn_sid, "test_descr1", "q")
	if err == nil {
		t.Log("procedure \"test_descr1\" should not be exists")
		t.Fail()
	}
}

const stm = `
create or replace procedure test_descr
  (
    ap1 in varchar2
    ,ap2 in string
    ,ap3 in char
    ,ap4 in number
    ,ap5 in owa.raw_arr
    ,ap6 in sys.dbms_describe.number_table
    ,ap7 in float
    ,ap8 in decimal
    ,ap9 in integer
    ,ap10 in pls_integer
    ,ap11 in boolean
    ,ap12 in owa.vc_arr
    ,ap13 in owa.nc_arr
    ,ap14 in da_agt.d%type
    ,ap15 in apex_040200.vc4000array
  ) 
is 
begin 
  null; 
end;`
