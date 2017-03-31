// auth
package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/vsdutka/iplsgo/auth/ntlm"
)

func Authenticator(authType int,
	authRealm, defUserName, defUserPass, authNTLMDBUserName, authNTLMDBUserPass string,
	grps map[int32]string,
	next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		switch authType {
		case 0 /*""*/ :
			r.Header.Add("X-AuthUserName", defUserName) //Так - фиктивными записями в header - передаём данные, чтобы не менять конструкции http.Request
			r.Header.Add("X-LoginUserName", defUserName)
			r.Header.Add("X-LoginPassword", defUserPass)

			isSpecial, connStr := getConnectionParams(defUserName, grps)
			if connStr == "" {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("Unauthorized"))
				return
			}
			r.Header.Add("X-LoginConnectionString", connStr)
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

				r.Header.Add("X-AuthUserName", userName) //Так - фиктивными записями в header - передаём данные, чтобы не менять конструкции http.Request
				r.Header.Add("X-LoginUserName", userName)
				r.Header.Add("X-LoginPassword", userPass)

				isSpecial, connStr := getConnectionParams(userName, grps)
				if connStr == "" {
					w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=\"%s%s\"", r.Host, authRealm))
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte("Unauthorized"))
					return
				}
				r.Header.Add("X-LoginConnectionString", connStr)
				r.Header.Add("X-LoginMany", strconv.FormatBool(isSpecial))

				w.Header().Set("Connection", "keep-alive")
				w.Header().Set("Persistent-Auth", "true")

				next(w, r, p)
			}
		case 2 /*"NTLM"*/ :
			{
				userName, ok, err := ntlm.Context().Authenticated(r.RemoteAddr)
				if err != nil {

					http.Error(w, "NTLM error: "+err.Error(), http.StatusInternalServerError)
					return
				}
				if !ok {
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

					if ok, err := ntlm.Context().Exists(r.RemoteAddr); !ok || err != nil {
						if err != nil {
							http.Error(w, "NTLM error: "+err.Error(), http.StatusInternalServerError)
							return
						}
						if !ok {

							var challenge string
							challenge, err = ntlm.Context().NewContext(r.RemoteAddr, authPayload)
							if err != nil {
								http.Error(w, "NTLM error: "+err.Error(), http.StatusInternalServerError)
								return
							}
							if challenge != "" {
								w.Header().Set("WWW-Authenticate", "NTLM "+challenge)
								http.Error(w, "Respond to challenge", http.StatusUnauthorized)
								return
							}
						}
					}
					userName, err = ntlm.Context().Authenticate(r.RemoteAddr, authPayload)
					if err != nil {
						http.Error(w, err.Error(), http.StatusUnauthorized)
						return
					}
					if userName == "" {
						initiateNTLM(w)
						return
					}

				}

				names := strings.Split(userName, "\\")
				if len(names) > 1 {
					userName = names[1]
				}
				r.Header.Add("X-AuthUserName", userName) //Так - фиктивными записями в header - передаём данные, чтобы не менять конструкции http.Request
				r.Header.Add("X-LoginUserName", authNTLMDBUserName)
				r.Header.Add("X-LoginPassword", authNTLMDBUserPass)

				isSpecial, connStr := getConnectionParams(userName, grps)
				if connStr == "" {
					w.Header().Set("WWW-Authenticate", fmt.Sprintf("NTLM realm=\"%s%s\"", r.Host, authRealm))
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte("Unauthorized"))
					return
				}
				r.Header.Add("X-LoginConnectionString", connStr)
				r.Header.Add("X-LoginMany", strconv.FormatBool(isSpecial))

				next(w, r, p)
			}
		}

	}
}

func initiateNTLM(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "NTLM")
	http.Error(w, "Authorization required", http.StatusUnauthorized)
	return
}
