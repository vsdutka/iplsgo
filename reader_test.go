// reader_test
package main

import (
	"flag"
	"gopkg.in/goracle.v1/oracle"
	"testing"
)

var (
	conf_dsn       = flag.String("conf-dsn", "", "Config Oracle DSN (user/passw@sid)")
	conf_dsn_user  string
	conf_dsn_passw string
	conf_dsn_sid   string
)

func init() {
	flag.Parse()
	conf_dsn_user, conf_dsn_passw, conf_dsn_sid = oracle.SplitDSN(*conf_dsn)
}

func TestReading(t *testing.T) {
	if !(*conf_dsn != "") {
		t.Fatalf("cannot test connection without dsn!")
	}
	err := startReading(*conf_dsn, "unionASR.xcfg", 1)
	if err != nil {
		t.Fatal(err)
	}
	stopReading()
	resetConfig()

	err = startReading(conf_dsn_user+"/"+conf_dsn_passw+"@"+conf_dsn_sid, "__unionASR.xcfg", 1)
	if err == nil {
		stopReading()
		t.Fatal("Should be error")
	}
	resetConfig()
	err = startReading(conf_dsn_user+"/"+conf_dsn_passw+"invalid"+"@"+conf_dsn_sid, "__unionASR.xcfg", 1)
	if err == nil {
		stopReading()
		t.Fatal("Should be error")
	}
	resetConfig()
}
