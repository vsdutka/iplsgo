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

type context struct {
	m             sync.Mutex
	sc            *ntlm_b.ServerContext
	userName      string
	authenticated bool
	expiried      time.Time
}

type contexts struct {
	m           sync.RWMutex
	contexts    map[string]*context
	serverCreds *sspi.Credentials
}

func (c *contexts) init() error {
	if c.serverCreds == nil {
		c.m.Lock()
		defer c.m.Unlock()
		var err error
		c.serverCreds, err = ntlm_b.AcquireServerCredentials()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *contexts) Exists(connId string) (bool, error) {
	if err := c.init(); err != nil {
		return false, err
	}
	c.m.RLock()
	defer c.m.RUnlock()
	_, ok := c.contexts[connId]
	return ok, nil
}
func (c *contexts) NewContext(connId string, negotiate []byte) (string, error) {
	if err := c.init(); err != nil {
		return "", err
	}

	sc, ch, err := func() (*ntlm_b.ServerContext, []byte, error) {
		c.m.RLock()
		defer c.m.RUnlock()
		return ntlm_b.NewServerContext(c.serverCreds, negotiate)

	}()
	if err != nil {
		return "", err
	}
	c.m.Lock()
	defer c.m.Unlock()
	newCtx, ok := ctxFree.Get().(*context)
	if !ok {
		newCtx = &context{
			sc:            sc,
			authenticated: false,
			expiried:      time.Now().Add(30 * time.Second),
		}
	} else {
		newCtx.sc = sc
		newCtx.authenticated = false
		newCtx.expiried = time.Now().Add(30 * time.Second)
	}
	c.contexts[connId] = newCtx
	ctxCounter.Add(1)
	return base64.StdEncoding.EncodeToString(ch), nil
}

func (c *contexts) Authenticate(connId string, authenticate []byte) (string, error) {
	if err := c.init(); err != nil {
		return "", err
	}

	c.m.RLock()
	defer c.m.RUnlock()
	cn, ok := c.contexts[connId]
	if !ok {
		return "", errors.New("Инициализированный контекст отсутствует")
	}

	cn.m.Lock()
	defer cn.m.Unlock()

	if err := cn.sc.Update(authenticate); err != nil {
		return "", err
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := cn.sc.ImpersonateUser(); err != nil {
		return "", err
	}
	defer cn.sc.RevertToSelf()
	un, err := getUserName()
	if err != nil {
		return "", err
	}
	cn.userName = un
	cn.expiried = time.Now().Add(30 * time.Second)
	cn.authenticated = true
	return un, nil
}

func getUserName() (string, error) {
	n := uint32(100)
	for {
		b := make([]uint16, n)
		e := syscall.GetUserNameEx(2, &b[0], &n)
		if e == nil {
			return syscall.UTF16ToString(b), nil
		}
		if e != syscall.ERROR_INSUFFICIENT_BUFFER {
			return "", e
		}
		if n <= uint32(len(b)) {
			return "", e
		}
	}
}

func (c *contexts) Authenticated(connId string) (string, bool, error) {
	if err := c.init(); err != nil {
		return "", false, err
	}

	c.m.RLock()
	defer c.m.RUnlock()
	cn, ok := c.contexts[connId]
	if !ok {
		return "", false, nil
	}

	cn.m.Lock()
	defer cn.m.Unlock()

	cn.expiried = time.Now().Add(30 * time.Second)
	return cn.userName, cn.authenticated, nil
}

func (c *contexts) cleanup() {
	c.m.Lock()
	defer c.m.Unlock()
	forClean := make([]string, 0)
	for k := range c.contexts {
		if c.contexts[k].expiried.Sub(time.Now()) < 0 {
			forClean = append(forClean, k)
		}
	}
	for _, id := range forClean {
		ctxFree.Put(c.contexts[id])
		delete(c.contexts, id)
		ctxCounter.Add(-1)
	}
}
