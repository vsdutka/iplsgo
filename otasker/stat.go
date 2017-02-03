// stat
package otasker

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

type taskStep struct {
	Name      string
	Duration  int32
	Statement string
}

type OracleTaskerStat struct {
	SortKey          string
	HandlerID        string
	MessageID        string
	Database         string
	UserName         string
	Password         string
	SessionID        string
	Created          string
	RequestProceeded int
	ErrorsNumber     int
	IdleTime         int32
	LastDuration     int32
	LastSteps        map[int]taskStep
	StepNum          int32
	StepName         string
	LastDocument     string
	LastProcedure    string
	NowInProcess     bool
}

type OracleTaskersStats []OracleTaskerStat

func (slice OracleTaskersStats) Len() int {
	return len(slice)
}

func (slice OracleTaskersStats) Less(i, j int) bool {
	return slice[i].SortKey < slice[j].SortKey
}

func (slice OracleTaskersStats) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func (r *oracleTasker) info(sortKeyName string) OracleTaskerStat {
	r.mt.Lock()
	defer r.mt.Unlock()

	processTime := int32(0)
	sSteps := make(map[int]taskStep)

	var keys []int
	for k := range r.logSteps {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	i := 1

	stepName := ""
	for _, v := range keys {
		val := r.logSteps[v]
		stepTime := int32(0)
		if (val.stepFn == time.Time{}) {
			stepName = val.stepName
			stepTime = int32(time.Since(val.stepBg) / time.Millisecond)
		} else {
			stepTime = int32(val.stepFn.Sub(val.stepBg) / time.Millisecond)
		}

		processTime = processTime + stepTime
		sSteps[i] = taskStep{Name: val.stepName, Duration: stepTime, Statement: val.stepStmForShowning}

		i = i + 1
	}

	idleTime := int32(time.Since(r.stateLastFinishDT) / time.Millisecond)
	if (r.stateLastFinishDT == time.Time{}) {
		idleTime = 0
	}

	res := OracleTaskerStat{
		"",
		r.logSessionID,
		r.logTaskID,
		r.logConnStr,
		r.logUserName,
		r.logUserPass,
		r.sessID,
		r.stateCreateDT.Format(time.RFC3339),
		r.logRequestProceeded,
		r.logErrorsNum,
		idleTime,
		processTime,
		sSteps,
		int32(len(sSteps) + 1),
		stepName,
		r.logProcName,
		r.logProcName,
		r.stateIsWorking,
	}
	rflct := reflect.ValueOf(res)
	f := reflect.Indirect(rflct).FieldByName(sortKeyName)

	switch k := f.Kind(); k {
	case reflect.Invalid:
		res.SortKey = "<invalid Value>"
	case reflect.String:
		res.SortKey = f.String()
	case reflect.Int, reflect.Int32, reflect.Int64:
		res.SortKey = fmt.Sprintf("%040d", f.Int())
	case reflect.Bool:
		res.SortKey = fmt.Sprintf("%v", f.Bool())
	}
	return res
}

func Collect(path, sortKeyName string, reversed bool) OracleTaskersStats {
	res := make(OracleTaskersStats, 0)
	wlock.RLock()
	defer wlock.RUnlock()
	for _, v := range wlist[strings.ToUpper(path)] {
		res = append(res, v.info(sortKeyName))
	}
	if reversed {
		sort.Sort(sort.Reverse(res))
	} else {
		sort.Sort(res)
	}
	return res
}
