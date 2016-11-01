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

// between request and reading response
func (r *Result) Wait() time.Duration {
	return r.beginread.Sub(r.endwrite)
}

// time to connect
func (r *Result) Connect() time.Duration {
	return r.connect.Sub(r.start)
}

// time for connection
func (r *Result) Total() time.Duration {
	return r.done.Sub(r.start)
}

type ResultList []*Result

var results ResultList

func (results ResultList) MinConnect() float64 {
	var min = math.MaxFloat64
	for _, r := range results {
		min = math.Min(min, float64(r.Connect()/time.Microsecond))
	}
	return min
}

func (results ResultList) MaxConnect() float64 {
	var max float64
	for _, r := range results {
		max = math.Max(max, float64(r.Connect()/time.Microsecond))
	}
	return max
}

func (results ResultList) TotalConnect() float64 {
	var sum float64
	for _, r := range results {
		sum += float64(r.Connect() / time.Microsecond)
	}
	return sum
}

func (results ResultList) MeanConnect(n int) float64 {
	return results.TotalConnect() / float64(n)
}

func (results ResultList) ConnectSD(n int) float64 {
	mean := results.MeanConnect(n)
	var variance float64
	for _, r := range results {
		d := float64(r.Connect()/time.Microsecond) - mean
		variance += math.Pow(d, 2)
	}
	if n > 1 {
		return math.Sqrt(variance / float64(n-1))
	} else {
		return 0
	}
}

func (results ResultList) ConnectMedian(n int) float64 {
	ctimes := results.MapCtime()
	sort.Float64s(ctimes)
	if n > 1 && (n%2) != 0 {
		return (ctimes[n/2] + ctimes[n/2+1]) / 2
	} else {
		return ctimes[n/2]
	}
}

func (results ResultList) MapCtime() []float64 {
	ctimes := make([]float64, len(results))
	for i, r := range results {
		ctimes[i] = float64(r.Connect() / time.Microsecond)
	}
	return ctimes
}

func GetUrl(requestUrl string) *Result {
	if b.startedCount >= *requests {
		return nil
	}
	b.IncrStarted()
	defer b.IncrDone()

	r := &Result{}

	trace := &httptrace.ClientTrace{
		ConnectStart: func(network, addr string) {
			//fmt.Println("ConnectStart:", r.start, network, addr)
		},
		GotConn: func(info httptrace.GotConnInfo) {
			r.connect = time.Now()
			b.SetLasttime(time.Now())
			//fmt.Printf("GotConn: %v %+v\n", r.connect, info)
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err != nil {
				fmt.Println("Failed to write the request", info.Err)
			}
			r.endwrite = time.Now()
			b.SetLasttime(time.Now())
			//fmt.Println("WroteRequest:", r.endwrite)
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

	r.start = time.Now()
	b.SetLasttime(time.Now())

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		b.IncrBad()
		fmt.Println(err)
		return nil
	}
	defer resp.Body.Close()

	r.beginread = time.Now()
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

	r.done = time.Now()
	b.SetLasttime(time.Now())

	return r
}

func Request(c chan string, r chan *Result) {
	for requestUrl := range c {
		r <- GetUrl(requestUrl)
	}
}

func SaveResult(ch chan *Result) {
	for r := range ch {
		if r != nil {
			results = append(results, r)
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
		RoundMillisecond(results.MinConnect()),
		RoundMillisecond(results.MeanConnect(b.doneCount)),
		RoundMillisecond(results.ConnectSD(b.doneCount)),
		RoundMillisecond(results.ConnectMedian(b.doneCount)),
		RoundMillisecond(results.MaxConnect()),
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
	go SaveResult(r)

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

	results = make(ResultList, 0, *concurrency)

	Test()
}
