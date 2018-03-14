// expvar
package metrics

import (
	"bytes"
	"fmt"
	"github.com/gorilla/websocket"
	"html/template"
	"io"
	"log"
	"math"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type update struct {
	Ts     int64
	Values map[string]string
}

type consumer struct {
	id uint
	c  chan update
}

type server struct {
	consumers      []consumer
	consumersMutex sync.RWMutex
}

const (
	maxCount int = 60 * 60 * 24 //86400
)

type Metric interface {
	gather(now int64) string
	writedata(w io.Writer, since int64)
}

type Int struct {
	sync.RWMutex
	i    int64
	data [][]int64
}

func (v *Int) gather(now int64) string {
	i := atomic.LoadInt64(&v.i)
	v.Lock()
	v.data = append(v.data, []int64{now, i})

	if len(v.data) > maxCount {
		v.data = v.data[len(v.data)-maxCount:]
	}
	v.Unlock()
	return strconv.FormatInt(i, 10)
}

func (v *Int) writedata(w io.Writer, since int64) {
	first := true
	w.Write([]byte("["))
	v.RLock()
	for _, item := range v.data {
		if item[0] > since {
			if first {
				first = false
			} else {
				w.Write([]byte(","))
			}
			fmt.Fprintf(w, "[%v, %v]", item[0], strconv.FormatInt(item[1], 10))
		}
	}
	v.RUnlock()
	w.Write([]byte("]"))
}

func (v *Int) Get() int64 {
	return atomic.LoadInt64(&v.i)
}

func (v *Int) Set(value int64) {
	atomic.StoreInt64(&v.i, value)
}

func (v *Int) Add(delta int64) {
	atomic.AddInt64(&v.i, delta)
}

func gather(now int64) {

}

type Float struct {
	sync.RWMutex
	f    uint64
	data [][]uint64
}

func (v *Float) gather(now int64) string {
	f := math.Float64frombits(atomic.LoadUint64(&v.f))
	v.Lock()
	v.data = append(v.data, []uint64{uint64(now), math.Float64bits(f)})

	if len(v.data) > maxCount {
		v.data = v.data[len(v.data)-maxCount:]
	}
	v.Unlock()
	return strconv.FormatFloat(f, 'g', -1, 64)
}
func (v *Float) writedata(w io.Writer, since int64) {
	first := true
	w.Write([]byte("["))
	v.RLock()
	for _, item := range v.data {
		if item[0] > uint64(since) {
			if first {
				first = false
			} else {
				w.Write([]byte(","))
			}
			fmt.Fprintf(w, "[%v, %v]", item[0], strconv.FormatFloat(math.Float64frombits(item[1]), 'g', -1, 64))
		}
	}
	v.RUnlock()
	w.Write([]byte("]"))
}

func (v *Float) Get() float64 {
	return math.Float64frombits(atomic.LoadUint64(&v.f))
}

func (v *Float) Set(value float64) {
	atomic.StoreUint64(&v.f, math.Float64bits(value))
}

func (v *Float) Add(delta float64) {
	for {
		cur := atomic.LoadUint64(&v.f)
		curVal := math.Float64frombits(cur)
		nxtVal := curVal + delta
		nxt := math.Float64bits(nxtVal)
		if atomic.CompareAndSwapUint64(&v.f, cur, nxt) {
			return
		}
	}
}

type metrics struct {
	mu          sync.RWMutex
	metricinfos map[string]struct {
		description   string
		unitname      string
		shortunitname string
	}
	metrics map[string]Metric
	keys    []string
}

// updateKeys updates the sorted list of keys in v.keys.
// must be called with v.mu held.
func (m *metrics) updateKeys() {
	if len(m.metrics) == len(m.keys) {
		// No new key.
		return
	}
	m.keys = m.keys[:0]
	for k := range m.metrics {
		m.keys = append(m.keys, k)
	}
	sort.Strings(v.keys)
}

func (m *metrics) gatherData() {
	memAlloc := NewInt("MemStats_Alloc", "General statistics - bytes allocated and not yet freed", "Bytes", "b")
	memTotalAlloc := NewInt("MemStats_TotalAlloc", "General statistics - bytes allocated (even if freed)", "Bytes", "b")
	memSys := NewInt("MemStats_Sys", "General statistics - bytes obtained from system (sum of XxxSys below)", "Bytes", "b")
	memLookups := NewInt("MemStats_Lookups", "General statistics - number of pointer lookups", "Pieces", "p")
	memMallocs := NewInt("MemStats_Mallocs", "General statistics - number of mallocs", "Pieces", "p")
	memFrees := NewInt("MemStats_Frees", "General statistics - number of frees", "Pieces", "p")

	memHeapAlloc := NewInt("MemStats_HeapAlloc", "Main allocation heap statistics - bytes allocated and not yet freed (same as Alloc above)", "Bytes", "b")
	memHeapSys := NewInt("MemStats_HeapSys", "Main allocation heap statistics - bytes obtained from system", "Bytes", "b")
	memHeapIdle := NewInt("MemStats_HeapIdle", "Main allocation heap statistics - bytes in idle spans", "Bytes", "b")
	memHeapInuse := NewInt("MemStats_HeapInuse", "Main allocation heap statistics - bytes in non-idle span", "Bytes", "b")
	memHeapReleased := NewInt("MemStats_HeapReleased", "Main allocation heap statistics - bytes released to the OS", "Bytes", "b")
	memHeapObjects := NewInt("MemStats_HeapObjects", "Main allocation heap statistics - total number of allocated objects", "Pieces", "p")

	memStackInuse := NewInt("MemStats_StackInuse", "LowLevel - bytes used by stack allocator - in use now", "Bytes", "b")
	memStackSys := NewInt("MemStats_StackSys", "LowLevel - bytes used by stack allocator  - obtained from sys", "Bytes", "b")
	memMSpanInuse := NewInt("MemStats_MSpanInuse", "LowLevel - mspan structures - in use now", "Bytes", "b")
	memMSpanSys := NewInt("MemStats_MSpanSys", "LowLevel - mspan structures - obtained from sys", "Bytes", "b")
	memMCacheInuse := NewInt("MemStats_MCacheInuse", "LowLevel - mcache structures - in use now", "Bytes", "b")
	memMCacheSys := NewInt("MemStats_MCacheSys", "LowLevel - mcache structures - obtained from sys", "Bytes", "b")
	memBuckHashSys := NewInt("MemStats_BuckHashSys", "LowLevel - profiling bucket hash table - obtained from sys", "Bytes", "b")
	memGCSys := NewInt("MemStats_GCSys", "LowLevel - GC metadata - obtained from sys", "Bytes", "b")
	memOtherSys := NewInt("MemStats_OtherSys", "LowLevel - other system allocations - obtained from sys", "Bytes", "b")

	memPauseNs := NewInt("MemStats_PauseNs", "GC pause duration", "Nanoseconds", "ns")
	numGoroutine := NewInt("Stats_NumGoroutine", "Number of goroutines", "Goroutines", "g")

	timer := time.Tick(time.Second)
	for {
		select {
		case now := <-timer:
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)

			memAlloc.Set(int64(ms.Alloc))
			memTotalAlloc.Set(int64(ms.TotalAlloc))
			memSys.Set(int64(ms.Sys))
			memLookups.Set(int64(ms.Lookups))
			memMallocs.Set(int64(ms.Mallocs))
			memFrees.Set(int64(ms.Frees))

			memHeapAlloc.Set(int64(ms.HeapAlloc))
			memHeapSys.Set(int64(ms.HeapSys))
			memHeapIdle.Set(int64(ms.HeapIdle))
			memHeapInuse.Set(int64(ms.HeapInuse))
			memHeapReleased.Set(int64(ms.HeapReleased))
			memHeapObjects.Set(int64(ms.HeapObjects))

			memStackInuse.Set(int64(ms.StackInuse))
			memStackSys.Set(int64(ms.StackSys))
			memMSpanInuse.Set(int64(ms.MSpanInuse))
			memMSpanSys.Set(int64(ms.MSpanSys))
			memMCacheInuse.Set(int64(ms.MCacheInuse))
			memMCacheSys.Set(int64(ms.MCacheSys))
			memBuckHashSys.Set(int64(ms.BuckHashSys))
			memGCSys.Set(int64(ms.GCSys))
			memOtherSys.Set(int64(ms.OtherSys))

			memPauseNs.Set(int64(ms.PauseNs[(ms.NumGC+255)%256]))
			numGoroutine.Set(int64(runtime.NumGoroutine()))

			func() {
				inow := now.Unix() * 1000
				u := update{
					Ts:     inow,
					Values: make(map[string]string),
				}

				func() {
					m.mu.RLock()
					defer m.mu.RUnlock()

					for _, k := range m.keys {
						mv := m.metrics[k]
						u.Values[k] = mv.gather(inow)
					}
				}()
				s.sendToConsumers(u)
			}()

		}
	}

}

var (
	v = metrics{
		metrics: make(map[string]Metric),
		metricinfos: make(map[string]struct {
			description   string
			unitname      string
			shortunitname string
		}),
		keys: make([]string, 0),
	}
	lastConsumerId uint
	s              server
	upgrader       = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

func publish(name, description, unitname, shortunitname string, m Metric) {
	v.mu.RLock()
	_, ok := v.metrics[name]
	v.mu.RUnlock()
	if ok {
		log.Panicln("Reuse of exported metric name:", name)
	}
	// check again under the write lock
	v.mu.Lock()
	v.metricinfos[name] = struct {
		description   string
		unitname      string
		shortunitname string
	}{
		description,
		unitname,
		shortunitname,
	}
	v.metrics[name] = m

	v.updateKeys()
	v.mu.Unlock()
}

func NewInt(name, description, unitname, shortunitname string) *Int {
	vI := &Int{}
	publish(name, description, unitname, shortunitname, vI)
	return vI
}

func NewFloat(name, description, unitname, shortunitname string) *Float {
	vF := &Float{}
	publish(name, description, unitname, shortunitname, vF)
	return vF
}

func Get(name string) Metric {
	v.mu.RLock()
	m, ok := v.metrics[name]
	v.mu.RUnlock()
	if !ok {
		log.Panicln("Unregistered metric:", name)
	}
	return m
}

func Add(name string, delta int64) {
	v.mu.RLock()
	m, ok := v.metrics[name]
	v.mu.RUnlock()
	if !ok {
		log.Panicln("Unregistered metric:", name)
	}
	if iv, ok := m.(*Int); ok {
		iv.Add(delta)
	}
}

func AddFloat(name string, delta float64) {
	v.mu.RLock()
	m, ok := v.metrics[name]
	v.mu.RUnlock()
	if !ok {
		log.Panicln("Unregistered metric:", name)
	}
	if fv, ok := m.(*Float); ok {
		fv.Add(delta)
	}
}

func (s *server) sendToConsumers(u update) {
	s.consumersMutex.RLock()
	defer s.consumersMutex.RUnlock()

	for _, c := range s.consumers {
		c.c <- u
	}
}

func (s *server) removeConsumer(id uint) {
	s.consumersMutex.Lock()
	defer s.consumersMutex.Unlock()

	var consumerId uint
	var consumerFound bool

	for i, c := range s.consumers {
		if c.id == id {
			consumerFound = true
			consumerId = uint(i)
			break
		}
	}

	if consumerFound {
		s.consumers = append(s.consumers[:consumerId], s.consumers[consumerId+1:]...)
	}
}

func (s *server) addConsumer() consumer {
	s.consumersMutex.Lock()
	defer s.consumersMutex.Unlock()

	lastConsumerId += 1

	c := consumer{
		id: lastConsumerId,
		c:  make(chan update),
	}

	s.consumers = append(s.consumers, c)

	return c
}

func (s *server) dataFeedHandler(w http.ResponseWriter, r *http.Request) {
	var (
		lastPing time.Time
		lastPong time.Time
	)

	varName := r.FormValue("var")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error: ", err)
		return
	}

	conn.SetPongHandler(func(s string) error {
		lastPong = time.Now()
		return nil
	})

	// read and discard all messages
	go func(c *websocket.Conn) {
		for {
			if _, _, err := c.NextReader(); err != nil {
				c.Close()
				break
			}
		}
	}(conn)

	c := s.addConsumer()

	defer func() {
		s.removeConsumer(c.id)
		conn.Close()
	}()

	var i uint

	for u := range c.c {
		var buffer bytes.Buffer
		buffer.WriteString("{\n")
		buffer.WriteString(fmt.Sprintf("\"Ts\": %v\n,\"Value\": %s", u.Ts, u.Values[varName]))
		buffer.WriteString("\n}\n")
		conn.WriteMessage(websocket.TextMessage, buffer.Bytes())

		i += 1

		if i%10 == 0 {
			if diff := lastPing.Sub(lastPong); diff > time.Second*60 {
				return
			}
			now := time.Now()
			if err := conn.WriteControl(websocket.PingMessage, nil, now.Add(time.Second)); err != nil {
				return
			}
			lastPing = now
		}
	}
}

func dataHandler(w http.ResponseWriter, r *http.Request) {
	if e := r.ParseForm(); e != nil {
		log.Fatalln("error pardsing form")
	}

	callback := r.FormValue("callback")
	fmt.Fprintf(w, "%v(", callback)

	w.Header().Set("Content-Type", "application/json")

	fmt.Fprintf(w, "{\n")

	fmt.Fprintf(w, "\"ts\": %v", time.Now().Unix()*1000)

	varName := r.FormValue("var")

	func() {
		v.mu.RLock()
		for k, _ := range v.metrics {
			if (varName == "") || (varName == k) {
				fmt.Fprintf(w, ",\n%q: ", k)
				v.metrics[k].writedata(w, 0)
			}
		}
		v.mu.RUnlock()
	}()
	fmt.Fprintf(w, "\n}\n")

	fmt.Fprint(w, ")")
}

type Var struct {
	VarName  string
	VarDesc  string
	Selected int
}
type Vars []Var

func (v Vars) Len() int { return len(v) }

func (v Vars) Swap(i, j int) { v[i], v[j] = v[j], v[i] }

func (v Vars) Less(i, j int) bool { return v[i].VarDesc < v[j].VarDesc }

func handleTemplate(templateName, templateBody string) func(http.ResponseWriter, *http.Request) {
	templ, err := template.New(templateName).Parse(templateBody)
	if err != nil {
		panic(err)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		v.mu.RLock()
		defer v.mu.RUnlock()
		type _T struct {
			Vars           []Var
			VarName        string
			VarDesc        string
			ValueUnit      string
			ValueUnitShort string
		}
		t := _T{VarName: r.FormValue("var"), Vars: make(Vars, 0)}

		i, ok := v.metricinfos[r.FormValue("var")]
		if !ok {
			t.VarDesc = r.FormValue("var")
			t.ValueUnit = ""
			t.ValueUnitShort = ""
		} else {
			t.VarDesc = i.description
			t.ValueUnit = i.unitname
			t.ValueUnitShort = i.shortunitname
		}

		for k, _ := range v.metricinfos {
			if k == t.VarName {
				t.Vars = append(t.Vars, Var{
					VarName:  k,
					VarDesc:  v.metricinfos[k].description,
					Selected: 1,
				})
			} else {
				t.Vars = append(t.Vars, Var{
					VarName:  k,
					VarDesc:  v.metricinfos[k].description,
					Selected: 0,
				})
			}
		}
		sort.Sort(Vars(t.Vars))

		if r.FormValue("var") == "" {
			t.Vars = append(t.Vars, Var{
				VarName:  "",
				VarDesc:  "",
				Selected: 1,
			})
		}

		err = templ.ExecuteTemplate(w, templateName, t)
		if err != nil {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(200)
			fmt.Fprint(w, "Error:", err)
			return
		}
	}
}

func init() {
	go v.gatherData()
	http.HandleFunc("/debug/metrics/data-feed", s.dataFeedHandler)
	http.HandleFunc("/debug/metrics/data", dataHandler)
	http.HandleFunc("/debug/metrics/main.html", handleTemplate("mainPage", mainPage))
	http.HandleFunc("/debug/metrics/main.js", handleTemplate("mainJs", mainJs))
}

const (
	mainPage = `
<!DOCTYPE html>
<html>
	<head>
		<title></title>
		<meta charset="utf-8" />
		<script src="http://code.jquery.com/jquery-1.11.0.min.js"></script>
		<script src="http://code.highcharts.com/stock/highstock.js"></script>
		<script src="http://code.highcharts.com/stock/modules/exporting.js"></script>
		<script>
		function doNavigate(varName){
			window.location.assign("/debug/metrics/main.html?var="+varName);
		}
		</script>
	</head>
	<body>
Select variable: <select onChange="javascript:doNavigate(this.value);">>
{{range $key, $data := .Vars}}
<option value="{{$data.VarName}}" {{if eq $data.Selected 1}}selected{{end}}>{{$data.VarDesc}}</option>
{{end}}
</select>
		<div id="container" style="min-width: 310px; height: 400px; margin: 0 auto"></div>
		<script src="main.js?var={{.VarName}}"></script>
	</body>
</html>`

	mainJs = `
var chartD;


$(function() {
	var x = new Date();

	Highcharts.setOptions({
		global: {
			timezoneOffset: x.getTimezoneOffset()
		}
	})

	$.getJSON('/debug/metrics/data?callback=?&var={{.VarName}}', function(data) {
		chartD = new Highcharts.StockChart({
			chart: {
				renderTo: 'container',
				zoomType: 'x'
			},
			title: {
				text: '{{.VarDesc}}'
			},
			yAxis: {
				title: {
					text: '{{.ValueUnit}}'
				}
			},
			scrollbar: {
				enabled: false
			},
			rangeSelector: {
				buttons: [{
					type: 'second',
					count: 5,
					text: '5s'
				}, {
					type: 'second',
					count: 30,
					text: '30s'
				}, {
					type: 'minute',
					count: 1,
					text: '1m'
				}, {
					type: 'all',
					text: 'All'
				}],
				selected: 3
			},
			series: [{
				name: "{{.VarDesc}}",
				data: data.{{.VarName}},
				type: 'area',
				tooltip: {
					valueSuffix: '{{.ValueUnitShort}}'
				}
			}]
		})
	});


	function wsurl() {
		var l = window.location;
		return ((l.protocol === "https:") ? "wss://" : "ws://") + l.hostname + (((l.port != 80) && (l.port != 443)) ? ":" + l.port : "") + "/debug/metrics/data-feed?var={{.VarName}}";
	}

	ws = new WebSocket(wsurl());
	ws.onopen = function () {
		ws.onmessage = function (evt) {
			var data = JSON.parse(evt.data);
			chartD.series[0].addPoint([data.Ts, data.Value], true);
		}
	};
})
`
)
