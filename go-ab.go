package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/http/httptrace"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var verbosity *int
var requests *int
var concurrency *int
var targetUrl *url.URL

var servername string
var hostname string
var port int
var path string
var doclen int

type Benchmark struct {
	start         time.Time
	lasttime      time.Time
	startedCount  int
	doneCount     int
	goodCount     int
	badCount      int
	noSucessCount int
	totalRead     int
	mux           sync.Mutex
}

func (b *Benchmark) SetLasttime(t time.Time) {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.lasttime = t
}

func (b *Benchmark) IncrStarted() {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.startedCount++
}

func (b *Benchmark) IncrDone() {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.doneCount++
}

func (b *Benchmark) IncrGood() {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.goodCount++
}

func (b *Benchmark) IncrBad() {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.badCount++
}

func (b *Benchmark) IncrNoSuccessCount() {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.noSucessCount++
}

func (b *Benchmark) AddTotalRead(size int) {
	b.mux.Lock()
	defer b.mux.Unlock()
	b.totalRead += size
}

func (b *Benchmark) TimeTaken() float64 {
	return b.lasttime.Sub(b.start).Seconds()
}

func (b *Benchmark) RequestPerSecond() float64 {
	return float64(b.doneCount) / b.TimeTaken()
}

func (b *Benchmark) TimePerRequest() float64 {
	return b.TimeTaken() * 1000 / float64(b.doneCount)
}

func (b *Benchmark) TransferRate() float64 {
	return float64(b.totalRead) / 1024 / b.TimeTaken()
}

var b = &Benchmark{}

type Result struct {
	start     time.Time // Start of connection
	connect   time.Time // Connected start writing
	endwrite  time.Time // Request written
	beginread time.Time // First byte of input
	done      time.Time // Connection closed
}

func (c *Result) WaitSecond() time.Duration {
	return c.beginread.Sub(c.endwrite)
}

func (c *Result) TimeToConnect() time.Duration {
	return c.connect.Sub(c.start)
}

func (c *Result) TotalSecond() time.Duration {
	return c.done.Sub(c.start)
}

type Stat struct {
	starttime time.Time     // start time of connection
	waittime  time.Duration // between request and reading response
	ctime     time.Duration // time to connect
	time      time.Duration // time for connection
}

type Stats []*Stat

var stats Stats

func (ss Stats) MinConnectTime() float64 {
	var min = math.MaxFloat64
	for _, s := range ss {
		min = math.Min(min, float64(s.ctime/time.Microsecond))
	}
	return min
}

func (ss Stats) MaxConnectTime() float64 {
	var max float64
	for _, s := range ss {
		max = math.Max(max, float64(s.ctime/time.Microsecond))
	}
	return max
}

func (ss Stats) TotalConnectTime() float64 {
	var sum float64
	for _, s := range ss {
		sum += float64(s.ctime / time.Microsecond)
	}
	return sum
}

func (ss Stats) MeanConnectTime(n int) float64 {
	return ss.TotalConnectTime() / float64(n)
}

func (ss Stats) ConnectTimeSD(n int) float64 {
	mean := ss.MeanConnectTime(n)
	var variance float64
	for _, s := range ss {
		d := float64(s.ctime/time.Microsecond) - mean
		variance += math.Pow(d, 2)
	}
	if n > 1 {
		return math.Sqrt(variance / float64(n-1))
	} else {
		return 0
	}
}

func (ss Stats) ConnectTimeMedian(n int) float64 {
	ctimes := ss.MapCtime()
	sort.Float64s(ctimes)
	if n > 1 && (n%2) != 0 {
		return (ctimes[n/2] + ctimes[n/2+1]) / 2
	} else {
		return ctimes[n/2]
	}
}

func (ss Stats) MapCtime() []float64 {
	r := make([]float64, len(ss))
	for i, s := range ss {
		r[i] = float64(s.ctime / time.Microsecond)
	}
	return r
}

func GetUrl(requestUrl string) *Result {
	if b.startedCount >= *requests {
		return nil
	}
	b.IncrStarted()
	defer b.IncrDone()

	c := &Result{}

	trace := &httptrace.ClientTrace{
		ConnectStart: func(network, addr string) {
			//fmt.Println("ConnectStart:", c.start, network, addr)
		},
		GotConn: func(info httptrace.GotConnInfo) {
			c.connect = time.Now()
			b.SetLasttime(time.Now())
			//fmt.Printf("GotConn: %v %+v\n", c.connect, info)
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err != nil {
				fmt.Println("Failed to write the request", info.Err)
			}
			c.endwrite = time.Now()
			b.SetLasttime(time.Now())
			//fmt.Println("WroteRequest:", c.endwrite)
		},
	}

	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		b.IncrBad()
		fmt.Println(err)
		return nil
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	// Disable keep-alive request for test
	req.Close = true

	c.start = time.Now()
	b.SetLasttime(time.Now())

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		b.IncrBad()
		fmt.Println(err)
		return nil
	}
	defer resp.Body.Close()

	c.beginread = time.Now()
	b.SetLasttime(time.Now())

	LogDebugf("Response code = %s\n", resp.Status)

	if !strings.HasPrefix(resp.Status, "2") {
		b.IncrNoSuccessCount()
	}

	headerDump, err := httputil.DumpResponse(resp, false)
	if err != nil {
		b.IncrBad()
		fmt.Println(err)
		return nil
	}

	servername = resp.Header.Get("Server")
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		b.IncrBad()
		fmt.Println(err)
		return nil
	}
	doclen = len(body)

	b.AddTotalRead(len(headerDump) + len(body))

	b.IncrGood()

	c.done = time.Now()
	b.SetLasttime(time.Now())

	return c
}

func Request(c chan string, r chan *Result) {
	for requestUrl := range c {
		r <- GetUrl(requestUrl)
	}
}

func SaveStats(r chan *Result) {
	for c := range r {
		if c != nil {
			s := &Stat{
				c.start,
				c.WaitSecond(),
				c.TimeToConnect(),
				c.TotalSecond()}
			stats = append(stats, s)
		}
	}
}

func OutputResults() {
	fmt.Printf("\n\n")
	fmt.Printf("Server Software:        %s\n", servername)
	fmt.Printf("Server Hostname:        %s\n", hostname)
	fmt.Printf("Server Port:            %d\n", port)
	fmt.Printf("\n")
	fmt.Printf("Document Path:          %s\n", path)
	fmt.Printf("Document Length:        %d bytes\n", doclen)
	fmt.Printf("\n")
	fmt.Printf("Concurrency Level:      %d\n", *concurrency)
	fmt.Printf("Time taken for tests:   %.3f seconds\n", b.TimeTaken())
	fmt.Printf("Complete requests:      %d\n", b.doneCount)
	fmt.Printf("Failed requests:        %d\n", b.badCount)
	if b.noSucessCount > 0 {
		fmt.Printf("Non-2xx responses:      %d\n", b.noSucessCount)
	}
	fmt.Printf("Total transferred:      %d bytes\n", b.totalRead)
	fmt.Printf("Requests per second:    %.2f [#/sec] (mean)\n", b.RequestPerSecond())
	fmt.Printf("Time per request:       %.3f [ms] (mean)\n", float64(*concurrency)*b.TimePerRequest())
	fmt.Printf("Time per request:       %.3f [ms] (mean, across all concurrent requests)\n", b.TimePerRequest())
	fmt.Printf("Transfer rate:          %.2f [Kbytes/sec] received\n", b.TransferRate())
	fmt.Printf("\n")
	fmt.Printf("Connection Times (ms)\n")
	fmt.Printf("              min  mean[+/-sd] median   max\n")
	fmt.Printf("Connect:    %5.0f %4.0f %5.1f %6.0f %7.0f\n",
		RoundMillisecond(stats.MinConnectTime()),
		RoundMillisecond(stats.MeanConnectTime(b.doneCount)),
		RoundMillisecond(stats.ConnectTimeSD(b.doneCount)),
		RoundMillisecond(stats.ConnectTimeMedian(b.doneCount)),
		RoundMillisecond(stats.MaxConnectTime()),
	)
}

func RoundMillisecond(s float64) float64 {
	return (s + 500) / 1000
}

func Test() {
	fmt.Printf("Benchmarking %s ", hostname)
	fmt.Printf("(be patient)%s", "...")

	b.start = time.Now()
	b.lasttime = time.Now()

	r := make(chan *Result)
	go SaveStats(r)

	ch := make([]chan string, *concurrency)
	for i := 0; i < *concurrency; i++ {
		ch[i] = make(chan string)
		go Request(ch[i], r)
	}

	for {
		for i := 0; i < *concurrency; i++ {
			ch[i] <- targetUrl.String()
		}
		if b.doneCount >= *requests {
			break
		}
	}

	//fmt.Printf("Finished %d requests\n", done)
	fmt.Printf("..done\n")
	OutputResults()
}

func LogDebugf(format string, args ...interface{}) {
	if *verbosity > 0 {
		log.Printf(format, args)
	}
}

func ParseUrl(rawurl string) error {
	u, err := url.ParseRequestURI(rawurl)
	if err != nil {
		return err
	}

	h := strings.Split(u.Host, ":")
	hostname = h[0]
	if len(h) > 1 {
		port, err = strconv.Atoi(h[1])
		if err != nil {
			return err
		}
	} else {
		if u.Scheme == "https" {
			port = 443
		} else {
			port = 80
		}
	}

	path = u.Path

	targetUrl = u

	return nil
}

func main() {
	verbosity = flag.Int("v", 0, "How much troubleshooting info to print")
	requests = flag.Int("n", 1, "Number of requests to perform")
	concurrency = flag.Int("c", 1, "Number of multiple requests to make at a time")
	flag.Parse()

	rawurl := flag.Arg(0)
	if rawurl == "" {
		fmt.Println("Usage: go-ab [http[s]://]hostname[:port]/path")
		return
	}

	err := ParseUrl(rawurl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: invalid URL\n", rawurl)
		// TODO: show usage
		return
	}

	if *requests < *concurrency {
		// TODO: show usage
		return
	}

	log.SetPrefix("LOG: ")
	log.SetFlags(0)

	stats = make(Stats, 0, *concurrency)

	Test()
}
