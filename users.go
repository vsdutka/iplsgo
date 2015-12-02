// users
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
)

type userInfo struct {
	IsSpecial bool
	GrpID     int32
}

func getUserInfo(name string) (bool, int32, bool) {
	ulock.RLock()
	defer ulock.RUnlock()
	u, ok := ulist[strings.ToUpper(name)]
	if !ok {
		return false, -1, false
	}
	return u.IsSpecial, u.GrpID, true
}

func getUsers() ([]byte, error) {
	ulock.RLock()
	defer ulock.RUnlock()
	return json.Marshal(ulist)
}

func updateUsers(users []byte) {
	ulock.RLock()
	needToParse := !bytes.Equal(prev, users)
	ulock.RUnlock()

	if needToParse {
		func() {
			ulock.Lock()
			defer ulock.Unlock()

			copy(prev, users)

			for k := range ulist {
				usersFree.Put(ulist[k])
				delete(ulist, k)
			}

			if len(users) == 0 {
				return
			}

			type _tUser struct {
				Name      string
				IsSpecial bool
				GRP_ID    int32
			}
			var t = []_tUser{}
			if err := json.Unmarshal(users, &t); err != nil {
				logError(err)
			}

			for k := range t {
				u, ok := usersFree.Get().(*userInfo)
				if !ok {
					u = &userInfo{
						IsSpecial: t[k].IsSpecial,
						GrpID:     t[k].GRP_ID,
					}
				} else {
					u.IsSpecial = t[k].IsSpecial
					u.GrpID = t[k].GRP_ID
				}
				ulist[strings.ToUpper(t[k].Name)] = u
			}
		}()
	}
}

var (
	ulock     sync.RWMutex
	ulist     = make(map[string]*userInfo)
	usersFree = sync.Pool{
		New: func() interface{} {
			return new(userInfo)
		},
	}
	prev []byte
)
