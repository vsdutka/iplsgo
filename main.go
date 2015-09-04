package main

import (
	"flag"
	"fmt"
	"github.com/davecheney/profile"
	"github.com/kardianos/service"
	_ "golang.org/x/tools/go/ssa"
	"gopkg.in/goracle.v1/oracle"
	"log"
	"os"
	"runtime"
	"sync"
	"time"
)

var (
	logger              service.Logger
	loggerLock          sync.Mutex
	srv                 *applicationServer
	srvConfig           *Config
	svcFlag             *string
	dsnFlag             *string
	confNameFlag        *string
	confReadTimeoutFlag *int
)

func logInfof(format string, a ...interface{}) error {
	loggerLock.Lock()
	defer loggerLock.Unlock()
	if logger != nil {
		return logger.Infof(format, a...)
	}
	return nil
}
func logError(v ...interface{}) error {
	loggerLock.Lock()
	defer loggerLock.Unlock()
	if logger != nil {
		return logger.Error(v)
	}
	return nil
}

// Program structures.
//  Define Start and Stop methods.
type program struct {
	exit chan struct{}
}

func (p *program) Start(s service.Service) error {
	if service.Interactive() {
		logInfof("Service \"%s\" is running in terminal.", srv.ServiceDispName())
	} else {
		logInfof("Service \"%s\" is running under service manager.", srv.ServiceDispName())
	}
	p.exit = make(chan struct{})

	// Start should not block. Do the actual work async.
	go p.run()
	return nil
}
func (p *program) run() {
	srv.Start()
	logInfof("Service \"%s\" is started.", srv.ServiceDispName())

	for {
		select {
		case <-p.exit:
			return
		}
	}
}
func (p *program) Stop(s service.Service) error {
	// Any work in Stop should be quick, usually a few seconds at most.
	logInfof("Service \"%s\" is stopping.", srv.ServiceDispName())
	srvConfig.Stop()
	srv.Stop()
	logInfof("Service \"%s\" is stopped.", srv.ServiceDispName())
	close(p.exit)
	return nil
}

// Service setup.
//   Define service config.
//   Create the service.
//   Setup the logger.
//   Handle service controls (optional).
//   Run the service.
func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	oracle.IsDebug = false

	flag.Usage = usage
	svcFlag = flag.String("service", "", fmt.Sprintf("Control the system service. Valid actions: %q\n", service.ControlAction))
	dsnFlag = flag.String("dsn", "", "    Oracle DSN (user/passw@sid)")
	confNameFlag = flag.String("conf", "", "   Configuration name")
	confReadTimeoutFlag = flag.Int("conf_tm", 10, "Configuration read timeout in seconds")
	flag.Parse()

	if (*confNameFlag == "") || (*dsnFlag == "") {
		usage()
		os.Exit(2)
	}

	cfg := profile.Config{
		CPUProfile:  true,
		ProfilePath: ".", // store profiles in current directory
	}

	defer profile.Start(&cfg).Stop()

	srv = newApplicationServer()
	//srv.Load()
	srvConfig = NewConfig(*dsnFlag, *confNameFlag, (time.Duration)(*confReadTimeoutFlag)*time.Second, srv.setServerConfig)

	svcConfig := &service.Config{
		Name:        srv.ServiceName(),
		DisplayName: srv.ServiceDispName(),
		Description: srv.ServiceDispName(),
		Arguments:   []string{fmt.Sprintf("-dsn=%s", *dsnFlag), fmt.Sprintf("-conf=%s", *confNameFlag), fmt.Sprintf("-conf_tm=%v", *confReadTimeoutFlag)},
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	errs := make(chan error, 5)
	func() {
		loggerLock.Lock()
		defer loggerLock.Unlock()
		logger, err = s.Logger(errs)
		if err != nil {
			log.Fatal(err)
		}
	}()

	go func() {
		for {
			err := <-errs
			if err != nil {
				log.Print(err)
			}
		}
	}()

	if len(*svcFlag) != 0 {
		err := service.Control(s, *svcFlag)
		if err != nil {
			log.Printf("Valid actions: %q\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}
	err = s.Run()
	if err != nil {
		logError(err)
	}
}

const usageTemplate = `iplsgo is OWA/APEX listener

Usage: iplsgo commands

The commands are:
`

func usage() {
	fmt.Fprintln(os.Stderr, usageTemplate)
	flag.PrintDefaults()
}
