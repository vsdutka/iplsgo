// reader_test
package main

import (
	"testing"
)

const (
	testSid = "dp-tst8"
)

func TestReading(t *testing.T) {
	err := startReading("iplsql_config/1@"+testSid, "unionASR.xcfg", 1)
	if err != nil {
		t.Fatal(err)
	}
	stopReading()
	resetConfig()

	err = startReading("iplsql_config/1@"+testSid, "__unionASR.xcfg", 1)
	if err == nil {
		stopReading()
		t.Fatal("Should be error")
	}
	resetConfig()
	err = startReading("iplsql_config/2@"+testSid, "__unionASR.xcfg", 1)
	if err == nil {
		stopReading()
		t.Fatal("Should be error")
	}
	resetConfig()
}
