// worker
package otasker

import (
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/vsdutka/metrics"
	"github.com/vsdutka/mltpart"
)

var numberOfSessions = metrics.NewInt("PersistentHandler_Number_Of_Sessions", "Server - Number of persistent sessions", "Pieces", "p")

type work struct {
	sessionID         string
	taskID            string
	reqUserName       string
	reqUserPass       string
	reqConnStr        string
	reqParamStoreProc string
	reqBeforeScript   string
	reqAfterScript    string
	reqDocumentTable  string
	reqCGIEnv         map[string]string
	reqProc           string
	reqParams         url.Values
	reqFiles          *mltpart.Form
	dumpFileName      string
}

type worker struct {
	oracleTasker
	sync.RWMutex
	signalChan  chan string
	inChan      chan work
	outChanList map[string]chan OracleTaskResult
	startedAt   time.Time
	started     bool
}

func (w *worker) start() {
	w.Lock()
	w.startedAt = time.Now()
	w.started = true
	w.Unlock()
}

func (w *worker) finish() {
	w.Lock()
	w.startedAt = time.Time{}
	w.started = false
	w.Unlock()
	w.signalChan <- ""
}

func (w *worker) outChan(taskID string) (chan OracleTaskResult, bool) {
	w.RLock()
	defer w.RUnlock()
	c, ok := w.outChanList[taskID]
	return c, ok
}

func (w *worker) worked() int64 {
	w.RLock()
	defer w.RUnlock()
	if w.started {
		return int64(time.Since(w.startedAt) / time.Second)
	}
	return 0
}

func (w *worker) listen(path, ID string, idleTimeout time.Duration) {
	w.signalChan <- ""
	defer func() {
		// Удаляем данный обработчик из списка доступных
		wlock.Lock()
		delete(wlist[path], ID)
		wlock.Unlock()
		w.CloseAndFree()
		w.Lock()
		for k, _ := range w.outChanList {
			delete(w.outChanList, k)
		}
		w.Unlock()
		numberOfSessions.Add(-1)
	}()

	//	timer := acquireTimer(idleTimeout)
	//	defer releaseTimer(timer)
	for {
		select {
		case wrk := <-w.inChan:
			{
				w.start()

				res := func() OracleTaskResult {
					return w.Run(wrk.sessionID,
						wrk.taskID,
						wrk.reqUserName,
						wrk.reqUserPass,
						wrk.reqConnStr,
						wrk.reqParamStoreProc,
						wrk.reqBeforeScript,
						wrk.reqAfterScript,
						wrk.reqDocumentTable,
						wrk.reqCGIEnv,
						wrk.reqProc,
						wrk.reqParams,
						wrk.reqFiles,
						wrk.dumpFileName)
				}()
				outChan, ok := w.outChan(wrk.taskID)
				if ok {
					outChan <- res
				}
				w.finish()
				if res.StatusCode == StatusRequestWasInterrupted {
					return
				}

			}
		case /*<-timer.C*/ <-time.After(idleTimeout):
			{
				return
			}
		}
	}
}

var (
	wlock sync.RWMutex
	wlist = make(map[string]map[string]*worker)
)

const (
	ClassicTasker = iota
	ApexTasker
	EkbTasker
)

var (
	taskerFactory = map[int]func() oracleTasker{
		ClassicTasker: NewOwaClassicProcTasker(),
		ApexTasker:    NewOwaApexProcTasker(),
		EkbTasker:     NewOwaEkbProcTasker(),
	}
)

func Run(
	path string,
	typeTasker int,
	sessionID,
	taskID,
	userName,
	userPass,
	connStr,
	paramStoreProc,
	beforeScript,
	afterScript,
	documentTable string,
	cgiEnv map[string]string,
	procName string,
	urlParams url.Values,
	reqFiles *mltpart.Form,
	//fn func() oracleTasker,
	waitTimeout, idleTimeout time.Duration,
	dumpFileName string,
) OracleTaskResult {
	w := func() *worker {
		wlock.RLock()
		w, ok := wlist[strings.ToUpper(path)][strings.ToUpper(sessionID)]
		wlock.RUnlock()
		if !ok {
			wlock.Lock()
			defer wlock.Unlock()

			w = &worker{
				oracleTasker: taskerFactory[typeTasker](),
				signalChan:   make(chan string, 1),
				inChan:       make(chan work),
				outChanList:  make(map[string]chan OracleTaskResult),
				startedAt:    time.Time{},
				started:      false,
			}
			if _, ok := wlist[strings.ToUpper(path)]; !ok {
				wlist[strings.ToUpper(path)] = make(map[string]*worker)
			}
			wlist[strings.ToUpper(path)][strings.ToUpper(sessionID)] = w
			go w.listen(strings.ToUpper(path), strings.ToUpper(sessionID), idleTimeout)
			numberOfSessions.Add(1)
		}
		return w
	}()

	// Проверяем, если результаты по задаче
	outChan, ok := w.outChan(taskID)
	if !ok {
		//		timer := acquireTimer(waitTimeout)
		//		defer releaseTimer(timer)
		//Если еще не было отправки, то проверяем на то, что можно отправит
		select {
		case <-w.signalChan:
			{
				//Удалось прочитать сигнал о незанятости вокера. Шлем задачу в него
				wrk := work{
					sessionID:         sessionID,
					taskID:            taskID,
					reqUserName:       userName,
					reqUserPass:       userPass,
					reqConnStr:        connStr,
					reqParamStoreProc: paramStoreProc,
					reqBeforeScript:   beforeScript,
					reqAfterScript:    afterScript,
					reqDocumentTable:  documentTable,
					reqCGIEnv:         cgiEnv,
					reqProc:           procName,
					reqParams:         urlParams,
					reqFiles:          reqFiles,
					dumpFileName:      dumpFileName,
				}
				outChan = make(chan OracleTaskResult, 1)
				w.Lock()
				w.outChanList[taskID] = outChan
				w.Unlock()
				w.inChan <- wrk

			}
		case /*<-timer.C*/ <-time.After(waitTimeout):
			{
				/* Сигнализируем о том, что идет выполнение другог запроса и нужно предложить прервать */
				return OracleTaskResult{StatusCode: StatusBreakPage, Duration: w.worked()}
			}
		}
	}
	//Читаем результаты
	return func() OracleTaskResult {
		//		timer := acquireTimer(waitTimeout)
		//		defer releaseTimer(timer)
		select {
		case res := <-outChan:
			w.Lock()
			delete(w.outChanList, taskID)
			w.Unlock()
			return res
		case /*<-timer.C*/ <-time.After(waitTimeout):
			{
				/* Сигнализируем о том, что идет выполнение этого запроса и нужно показать червяка */
				return OracleTaskResult{StatusCode: StatusWaitPage, Duration: w.worked()}
			}
		}
	}()
}

func Break(path, sessionID string) error {
	wlock.RLock()
	w, ok := wlist[strings.ToUpper(path)][strings.ToUpper(sessionID)]
	wlock.RUnlock()
	if !ok {
		return nil
	}
	//Если вокер есть, проверяемего статус
	select {
	case <-w.signalChan:
		{
			//Удалось прочитать сигнал о незанятости вокера. Некого прерыват. Выходим
			return nil

		}
	default:
		{
			// Воркер занят. Прерываем его
			return w.Break()
		}
	}

}
