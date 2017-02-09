// auth
package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/alexbrainman/sspi"
	"github.com/alexbrainman/sspi/ntlm"
	"github.com/julienschmidt/httprouter"
)

func Authenticator(authType int, authRealm, defUserName, defUserPass string, grps map[int32]string, next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		switch authType {
		case 0 /*""*/ :
			r.Header.Add("X-AuthUserName", defUserName)
			r.Header.Add("X-LoginUserName", defUserName)
			r.Header.Add("X-LoginPassword", defUserPass)

			isSpecial, connStr := getConnectionParams(defUserName, grps)
			if connStr == "" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("Unauthorized"))
				return
			}
			r.Header.Add("X-LoginConnextionString", connStr)
			r.Header.Add("X-LoginMany", strconv.FormatBool(isSpecial))

			next(w, r, p)
			return
		case 1 /*"Basic"*/ :
			{
				userName, userPass, ok := r.BasicAuth()
				if !ok {
					w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"%s%s\"", r.Host, authRealm))
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte("Unauthorized"))
					return
				}

				r.Header.Add("X-AuthUserName", userName)
				r.Header.Add("X-LoginUserName", userName)
				r.Header.Add("X-LoginPassword", userPass)

				isSpecial, connStr := getConnectionParams(userName, grps)
				if connStr == "" {
					w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"%s%s\"", r.Host, authRealm))
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte("Unauthorized"))
					return
				}
				r.Header.Add("X-LoginConnextionString", connStr)
				r.Header.Add("X-LoginMany", strconv.FormatBool(isSpecial))

				w.Header().Set("Connection", "keep-alive")
				w.Header().Set("Persistent-Auth", "true")

				next(w, r, p)
			}
		case 2 /*"NTLM"*/ :
			{
				xSessiobId := r.Header.Get("X-Connection-ID")
				if xSessiobId != "" {

				}

				var err error
				auth := r.Header.Get("Authorization")
				if auth == "" || (len(strings.SplitN(auth, " ", 2)) < 2) {
					initiateNTLM(w)
					return
				}
				parts := strings.SplitN(auth, " ", 2)
				authType := parts[0]
				if authType != "NTLM" {
					initiateNTLM(w)
					return
				}
				var authPayload []byte
				authPayload, err = base64.StdEncoding.DecodeString(parts[1])
				context, ok := contexts[r.RemoteAddr]
				if !ok {
					sendChallenge(authPayload, w, r)
					return
				}
				defer delete(contexts, r.RemoteAddr)
				var userName string
				userName, err = authenticate(context, authPayload)
				if err != nil {
					http.Error(w, err.Error(), http.StatusUnauthorized)
					return
				}
				names := strings.Split(userName, "\\")
				if len(names) > 1 {
					userName = names[1]
				}
				r.Header.Add("X-AuthUserName", userName)
				r.Header.Add("X-LoginUserName", defUserName)
				r.Header.Add("X-LoginPassword", defUserPass)

				isSpecial, connStr := getConnectionParams(userName, grps)
				if connStr == "" {
					w.Header().Set("WWW-Authenticate", fmt.Sprintf("NTLM realm=\"%s%s\"", r.Host, authRealm))
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte("Unauthorized"))
					return
				}
				r.Header.Add("X-LoginConnextionString", connStr)
				r.Header.Add("X-LoginMany", strconv.FormatBool(isSpecial))

				w.Header().Set("X-Connection-ID", "identity")

				w.Header().Set("Transfer-Encoding", "identity")

				//w.Header().Set("Connection", "keep-alive")
				//w.Header().Set("Persistent-Auth", "true")
				//w.Header().Set("Connection", "close")

				next(w, r, p)
			}
		}

	}
}

var (
	contexts    map[string]*ntlm.ServerContext
	serverCreds *sspi.Credentials
)

func init() {
	contexts = make(map[string]*ntlm.ServerContext)

	var err error
	serverCreds, err = ntlm.AcquireServerCredentials()
	if err != nil {
		panic(err)
	}
}

func initiateNTLM(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "NTLM")
	http.Error(w, "Authorization required", http.StatusUnauthorized)
	return
}

func authenticate(c *ntlm.ServerContext, authenticate []byte) (userName string, err error) {
	defer c.Release()
	err = c.Update(authenticate)
	if err != nil {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	err = c.ImpersonateUser()
	if err != nil {
		return
	}
	defer c.RevertToSelf()
	return GetUserName()
}

func GetUserName() (string, error) {
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

func sendChallenge(negotiate []byte, w http.ResponseWriter, r *http.Request) {
	sc, ch, err := ntlm.NewServerContext(serverCreds, negotiate)
	if err != nil {
		http.Error(w, "NTLM error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	//FIXME 1 - Одного IP недостаточно для идентификации сессии, 2 - требуется процесс, очищающий кешь контекстов
	contexts[r.RemoteAddr] = sc
	w.Header().Set("WWW-Authenticate", "NTLM "+base64.StdEncoding.EncodeToString(ch))
	http.Error(w, "Respond to challenge", http.StatusUnauthorized)
	return
}
