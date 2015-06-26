package main

import (
	"flag"
	"github.com/kardianos/service"
	//_ "github.com/mkevac/debugcharts"
	"fmt"
	"gopkg.in/goracle.v1/oracle"
	"log"
	"runtime"
)

var logger service.Logger
var (
	srv          *applicationServer
	svcFlag      *string
	dsnFlag      *string
	confNameFlag *string
)

// Program structures.
//  Define Start and Stop methods.
type program struct {
	exit chan struct{}
}

func (p *program) Start(s service.Service) error {
	if service.Interactive() {
		logger.Infof("Service \"%s\" is running in terminal.", srv.ServiceDispName())
	} else {
		logger.Infof("Service \"%s\" is running under service manager.", srv.ServiceDispName())
	}
	p.exit = make(chan struct{})

	// Start should not block. Do the actual work async.
	go p.run()
	return nil
}
func (p *program) run() {
	srv.Start()
	logger.Infof("Service \"%s\" is started.", srv.ServiceDispName())

	for {
		select {
		case <-p.exit:
			return
		}
	}
}
func (p *program) Stop(s service.Service) error {
	// Any work in Stop should be quick, usually a few seconds at most.
	logger.Infof("Service \"%s\" is stopping.", srv.ServiceDispName())
	srv.Stop()
	logger.Infof("Service \"%s\" is stopped.", srv.ServiceDispName())
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
	svcFlag = flag.String("service", "", "Control the system service.")
	dsnFlag = flag.String("dsn", "", "Oracle DSN (user/passw@sid)")
	confNameFlag = flag.String("conf", "", "Configuration name")
	flag.Parse()

	srv = newApplicationServer()
	srv.Load()

	svcConfig := &service.Config{
		Name:        srv.ServiceName(),
		DisplayName: srv.ServiceDispName(),
		Description: srv.ServiceDispName(),
		Arguments:   []string{fmt.Sprintf("-dsn=%s", *dsnFlag), fmt.Sprintf("-conf=%s", *confNameFlag)},
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	errs := make(chan error, 5)
	logger, err = s.Logger(errs)
	if err != nil {
		log.Fatal(err)
	}

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
		logger.Error(err)
	}
}
