package main

//go:generate C:\!Dev\GOPATH\src\github.com\vsdutka\gover\gover.exe
import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	//_ "golang.org/x/tools/go/ssa"
	"gopkg.in/goracle.v1/oracle"
)

//ВАЖНО - собирать с GODEBUG=cgocheck=0
var (
	verFlag             *bool
	dsnFlag             *string
	hostFlag            *string
	confNameFlag        *string
	confReadTimeoutFlag *int
	healthy             int32
)

func logInfof(format string, a ...interface{}) error {
	// loggerLock.Lock()
	// defer loggerLock.Unlock()
	// if logger != nil {
	// 	return logger.Infof(format, a...)
	// }
	log.Printf(format, a...)
	return nil
}
func logError(v ...interface{}) error {
	// loggerLock.Lock()
	// defer loggerLock.Unlock()
	// if logger != nil {
	// 	return logger.Error(v)
	// }
	log.Println(v...)
	return nil
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	oracle.IsDebug = false

	flag.Usage = usage
	verFlag = flag.Bool("version", false, "Show version")
	dsnFlag = flag.String("dsn", "", "    Oracle DSN (user/passw@sid)")
	hostFlag = flag.String("host", "", "   Host name")
	confNameFlag = flag.String("conf", "", "   Configuration name")
	confReadTimeoutFlag = flag.Int("conf_tm", 10, "Configuration read timeout in seconds")
	flag.Parse()

	if *verFlag == true {
		log.Println("Version: ", VERSION)
		log.Println("Build:   ", BUILD_DATE)
		os.Exit(0)
	}

	if (*confNameFlag == "") || (*dsnFlag == "") {
		usage()
		os.Exit(2)
	}

	err := startReading(*dsnFlag, *confNameFlag, (time.Duration)(*confReadTimeoutFlag)*time.Second)
	if err != nil {
		log.Fatal(err)
	}

	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	signal.Notify(quit, os.Interrupt, syscall.SIGTRAP)

	go func() {
		<-quit
		logInfof("Server is shutting down...\n")
		atomic.StoreInt32(&healthy, 0)

		stopReading()
		stopServer()

		// ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		// defer cancel()

		// server.SetKeepAlivesEnabled(false)
		// if err := server.Shutdown(ctx); err != nil {
		// 	logger.Fatalf("Could not gracefully shutdown the server: %v\n", err)
		// }
		close(done)
	}()

	atomic.StoreInt32(&healthy, 1)

	startServer()
	logInfof("Service \"%s\" is started.\n", confServiceDispName)

	<-done
	logInfof("Server stopped\n")

}

const usageTemplate = `iplsgo is OWA/APEX listener

Usage: iplsgo commands

The commands are:
`

func usage() {
	fmt.Fprintln(os.Stderr, usageTemplate)
	flag.PrintDefaults()
}
