// confReader_test
package main

import (
	"flag"
	"fmt"
	//"os"
	"runtime"
	//"runtime/pprof"
	"encoding/json"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"testing"
	"time"
)

var dsn = flag.String("dsn", "", "Oracle DSN (user/passw@sid)")
var confName = flag.String("conf", "", "Configuration name")

func init() {
	flag.Parse()
}

type serverMok struct {
	wg sync.WaitGroup
	i  int
}

func (m *serverMok) serverCallback(
	serviceName, serviceDispName string,
	httpPort, httpDebugPort, httpReadTimeout, httpWriteTimeout int,
	httpSsl bool, httpSslCert, httpSslKey,
	httpLogDir string,
	handlersConfig []*json.RawMessage,
) error {
	type _t struct {
		Path   string `json:"Path"`
		Type   string `json:"Type"`
		Delete bool   `json:"Delete"`
	}
	t := _t{Delete: false}
	for _, v := range handlersConfig {
		if err := json.Unmarshal(*v, &t); err != nil {
			return err
		} else {
			//fmt.Println(t.Type, t.Path)
		}
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	fmt.Println(m.i, ",", mem.Alloc, ",", mem.TotalAlloc, ",", mem.HeapAlloc, ",", mem.HeapSys)
	m.i++
	if m.i == 100 {
		m.wg.Done()
	}
	return nil
}

func TestConfigReader(t *testing.T) {
	go http.ListenAndServe(":8888", nil)

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	fmt.Println("Alloc, TotalAlloc, HeapAlloc, HeapSys")
	fmt.Println(0, ",", mem.Alloc, ",", mem.TotalAlloc, ",", mem.HeapAlloc, ",", mem.HeapSys)
	//	fmt.Println("Alloc      - ", mem.Alloc)
	//	fmt.Println("TotalAlloc - ", mem.TotalAlloc)
	//	fmt.Println("HeapAlloc  - ", mem.HeapAlloc)
	//	fmt.Println("HeapSys    - ", mem.HeapSys)
	//	fmt.Println("-------------------------------")

	sm := serverMok{}
	sm.wg.Add(1)
	r := newConfigReader(*dsn, *confName, 5*time.Millisecond, "testConfigLogger.log", sm.serverCallback)
	sm.wg.Wait()
	r.shutdown()

	//	f, err := os.Create("memprof.log")
	//	if err != nil {
	//		t.Fatal(err)
	//	}
	//	pprof.WriteHeapProfile(f)
	//	f.Close()

}
