// soap
package main

import (
	"fmt"
	"github.com/julienschmidt/httprouter"
	"gopkg.in/errgo.v1"
	"gopkg.in/goracle.v1/oracle"
	"io/ioutil"
	"net/http"
	"path"
	"path/filepath"
)

func newSoap(pathStr string, userName, userPass, connStr string,
) func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {

	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		if r.Method != "POST" {
			writeError(w, http.StatusMethodNotAllowed, "soap: POST method required, received "+r.Method)
			return
		}

		outBuf, err := func() ([]byte, error) {

			soapAction := r.Header.Get("SOAPAction")
			if soapAction == "" {
				return nil, errgo.New("soap: SOAPAction required")
			}
			buf, err := ioutil.ReadAll(r.Body)
			if err != nil {
				return nil, errgo.New("soap: Body required, but error: " + err.Error())
			}
			if len(buf) == 0 {
				return nil, errgo.New("soap: Body required")
			}
			var (
				conn   *oracle.Connection
				cur    *oracle.Cursor
				inVar  *oracle.Variable
				outBuf []byte
				outVar *oracle.Variable
			)
			conn, err = oracle.NewConnection(userName, userPass, connStr, true)
			defer conn.Close()
			if err != nil {
				return nil, errgo.Newf("soap: Error connecting to DB: %s", err)
			}

			_, procName := filepath.Split(path.Clean(r.URL.Path))
			cur = conn.NewCursor()
			defer cur.Close()

			inVar, err = cur.NewVariable(0, oracle.ClobVarType, uint(len(buf)))
			if err != nil {
				return nil, errgo.Newf("soap: Error prepare variable: %s", err)
			}

			err = inVar.SetValue(0, buf)
			if err != nil {
				return nil, errgo.Newf("soap: Error setting input variable value: %s", err)
			}

			outVar, err = cur.NewVariable(0, oracle.ClobVarType, 0)
			if err != nil {
				return nil, errgo.Newf("soap: Error prepare variable: %s", err)
			}
			if err = cur.Execute(fmt.Sprintf(stm, procName), []interface{}{inVar, outVar}, nil); err != nil {
				return nil, errgo.Newf("soap: Error exec statement: %s", err)
			}
			outData, err1 := outVar.GetValue(0)
			if err1 != nil {
				return nil, errgo.Newf("soap: Error read response: %s", err1)
			}
			ext, ok := outData.(*oracle.ExternalLobVar)
			if !ok {
				return nil, errgo.Newf("soap: Error read response: %s", "Invalid variable type")
			}
			if ext != nil {
				size, err := ext.Size(false)
				if err != nil {
					fmt.Println("size error: ", err)
				}
				if size != 0 {
					outBuf, err = ext.ReadAll()
					if err != nil {
						return nil, errgo.Newf("soap: Error read response: %s", err)
					}
				}
			}
			return outBuf, nil
		}()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
		w.Write(outBuf)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, msg)
}

const stm = `DECLARE t CLOB := EMPTY_CLOB(); BEGIN t := %s(:1); :2 := t; dbms_session.modify_package_state(dbms_session.reinitialize);END;`
