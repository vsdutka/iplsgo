// users
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
)

type userInfo struct {
	isSpecial bool
	grpId     int32
}

func GetUserInfo(name string) (bool, int32, bool) {
	ulock.RLock()
	defer ulock.RUnlock()
	u, ok := ulist[strings.ToUpper(name)]
	if !ok {
		return false, -1, false
	}
	return u.isSpecial, u.grpId, true
}

func UpdateUsers(users *[]byte) {
	ulock.RLock()
	needToParse := !bytes.Equal(prev, *users)
	ulock.RUnlock()

	if needToParse {
		func() {
			ulock.Lock()
			defer ulock.Unlock()

			copy(prev, *users)

			for k, _ := range ulist {
				usersFree.Put(ulist[k])
				delete(ulist, k)
			}

			type _tUser struct {
				Name      string
				IsSpecial bool
				GRP_ID    int32
			}
			t := make([]_tUser, 0)
			if err := json.Unmarshal(*users, &t); err != nil {
				logError(err)
			} else {
			}

			for k, _ := range t {
				u, ok := usersFree.Get().(*userInfo)
				if !ok {
					u = &userInfo{
						isSpecial: t[k].IsSpecial,
						grpId:     t[k].GRP_ID,
					}
				} else {
					u.isSpecial = t[k].IsSpecial
					u.grpId = t[k].GRP_ID
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
