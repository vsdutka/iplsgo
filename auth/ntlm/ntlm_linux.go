// ntlm
package ntlm

import (
	"fmt"
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

var (
	ctx = contexts{}
)

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

