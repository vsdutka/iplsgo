// main_common
package main

import (
	"flag"
	"fmt"
	"os"
)

var (
	verFlag             *bool
	dsnFlag             *string
	hostFlag            *string
	confNameFlag        *string
	confReadTimeoutFlag *int
	conectionString     *string
)

func setupFlags() {
	flag.Usage = usage
	verFlag = flag.Bool("version", false, "Show version")
	dsnFlag = flag.String("dsn", "", "    Oracle DSN (user/passw@sid)")
	hostFlag = flag.String("host", "", "   Host name")
	confNameFlag = flag.String("conf", "", "   Configuration name")
	confReadTimeoutFlag = flag.Int("conf_tm", 10, "Configuration read timeout in seconds")
	conectionString = flag.String("cs", "", "    Connection string for ALL users")
}

const usageTemplate = `iplsgo is OWA/APEX listener

Usage: iplsgo commands

The commands are:
`

func usage() {
	fmt.Fprintln(os.Stderr, usageTemplate)
	flag.PrintDefaults()
}
