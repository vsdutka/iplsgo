// ntlm
package ntlm

import (
	"encoding/base64"
	"errors"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/alexbrainman/sspi"
	ntlm_b "github.com/alexbrainman/sspi/ntlm"
	"github.com/vsdutka/metrics"
)

type AuthContext interface {
	Exists(connId string) (bool, error)
	NewContext(connId string, negotiate []byte) (string, error)
	Authenticate(connId string, authenticate []byte) (string, error)
	Authenticated(connId string) (string, bool, error)
	//	CreateContext(id string) (string, error)
	//	DeleteContext(id string) error

}

func Context() AuthContext {
	return &ctx
}

var ctxCounter = metrics.NewInt("ntlm_contexts", "NTLM - Number of contexts", "", "")

var (
	ctx = contexts{
		contexts:    make(map[string]*context),
		serverCreds: nil,
	}

	ctxFree = sync.Pool{
		New: func() interface{} {
			return new(context)
		},
	}
)

func init() {
	go func() {
		for {
			select {
			case <-time.After(time.Second * 1):
				{
					ctx.cleanup()
				}
			}
		}
	}()
}

type context struct {}

type contexts struct {}


func (c *contexts) Exists(connId string) (bool, error) {
	return false, fmt.Errorf("Unsupported");
}
func (c *contexts) NewContext(connId string, negotiate []byte) (string, error) {
	return "", fmt.Errorf("Unsupported");
}

func (c *contexts) Authenticate(connId string, authenticate []byte) (string, error) {
	return "", fmt.Errorf("Unsupported");
}

func getUserName() (string, error) {
	return "", fmt.Errorf("Unsupported");
}

func (c *contexts) Authenticated(connId string) (string, bool, error) {
	return "", false, fmt.Errorf("Unsupported");
}

