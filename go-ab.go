package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/takatoshiono/go-ab/stats"
	"github.com/tcnksm/go-httpstat"
)

var verbosity *int
var requests *int
var concurrency *int
var keepalive *bool
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
	done     time.Time
	httpstat httpstat.Result
}

func (r *Result) ContentTransfer() time.Duration {
	return r.httpstat.ContentTransfer(r.done)
}

func (r *Result) Total() time.Duration {
	return r.httpstat.Total(r.done)
}

type ResultList []*Result

var results ResultList

func (results ResultList) Durations() map[string][]float64 {
	d := map[string][]float64{
		"DNSLookup":        make([]float64, len(results)),
		"TCPConnection":    make([]float64, len(results)),
		"TLSHandshake":     make([]float64, len(results)),
		"ServerProcessing": make([]float64, len(results)),
		"ContentTransfer":  make([]float64, len(results)),
		"Total":            make([]float64, len(results)),
	}
	for i, r := range results {
		d["DNSLookup"][i] = float64(r.httpstat.DNSLookup / time.Microsecond)
		d["TCPConnection"][i] = float64(r.httpstat.TCPConnection / time.Microsecond)
		d["TLSHandshake"][i] = float64(r.httpstat.TLSHandshake / time.Microsecond)
		d["ServerProcessing"][i] = float64(r.httpstat.ServerProcessing / time.Microsecond)
		d["ContentTransfer"][i] = float64(r.ContentTransfer() / time.Microsecond)
		d["Total"][i] = float64(r.Total() / time.Microsecond)
	}
	return d
}

func GetUrl(requestUrl string) *Result {
	if b.startedCount >= *requests {
		return nil
	}
	b.IncrStarted()
	defer b.IncrDone()

	r := &Result{}

	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		b.IncrBad()
		fmt.Println(err)
		return nil
	}

	req = req.WithContext(httpstat.WithHTTPStat(req.Context(), &r.httpstat))
	if !*keepalive {
		req.Close = true
	}

	b.SetLasttime(time.Now())

	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		b.IncrBad()
		fmt.Println(err)
		return nil
	}
	defer resp.Body.Close()

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

	r.httpstat.End(time.Now())
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
	durations := results.Durations()

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
	fmt.Printf("                     min  mean[+/-sd] median   max\n")

	formatString := "%5.0f %4.0f %5.1f %6.0f %7.0f"
	fmt.Printf("DNSLookup:         "+formatString+"\n",
		RoundMillisecond(stats.Min(durations["DNSLookup"])),
		RoundMillisecond(stats.Mean(durations["DNSLookup"])),
		RoundMillisecond(stats.StandardDeviation(durations["DNSLookup"])),
		RoundMillisecond(stats.Median(durations["DNSLookup"])),
		RoundMillisecond(stats.Max(durations["DNSLookup"])),
	)
	fmt.Printf("TCPConnection:     "+formatString+"\n",
		RoundMillisecond(stats.Min(durations["TCPConnection"])),
		RoundMillisecond(stats.Mean(durations["TCPConnection"])),
		RoundMillisecond(stats.StandardDeviation(durations["TCPConnection"])),
		RoundMillisecond(stats.Median(durations["TCPConnection"])),
		RoundMillisecond(stats.Max(durations["TCPConnection"])),
	)
	fmt.Printf("TLSHandshake:      "+formatString+"\n",
		RoundMillisecond(stats.Min(durations["TLSHandshake"])),
		RoundMillisecond(stats.Mean(durations["TLSHandshake"])),
		RoundMillisecond(stats.StandardDeviation(durations["TLSHandshake"])),
		RoundMillisecond(stats.Median(durations["TLSHandshake"])),
		RoundMillisecond(stats.Max(durations["TLSHandshake"])),
	)
	fmt.Printf("ServerProcessing:  "+formatString+"\n",
		RoundMillisecond(stats.Min(durations["ServerProcessing"])),
		RoundMillisecond(stats.Mean(durations["ServerProcessing"])),
		RoundMillisecond(stats.StandardDeviation(durations["ServerProcessing"])),
		RoundMillisecond(stats.Median(durations["ServerProcessing"])),
		RoundMillisecond(stats.Max(durations["ServerProcessing"])),
	)
	fmt.Printf("ContentTransfer:   "+formatString+"\n",
		RoundMillisecond(stats.Min(durations["ContentTransfer"])),
		RoundMillisecond(stats.Mean(durations["ContentTransfer"])),
		RoundMillisecond(stats.StandardDeviation(durations["ContentTransfer"])),
		RoundMillisecond(stats.Median(durations["ContentTransfer"])),
		RoundMillisecond(stats.Max(durations["ContentTransfer"])),
	)
	fmt.Printf("Total:             "+formatString+"\n",
		RoundMillisecond(stats.Min(durations["Total"])),
		RoundMillisecond(stats.Mean(durations["Total"])),
		RoundMillisecond(stats.StandardDeviation(durations["Total"])),
		RoundMillisecond(stats.Median(durations["Total"])),
		RoundMillisecond(stats.Max(durations["Total"])),
	)
	fmt.Printf("\n")
	fmt.Printf("Percentage of the requests served within a certain time (ms)")
	fmt.Printf("\n")

	sort.Float64s(durations["Total"])
	percentages := [9]int{50, 66, 75, 80, 90, 95, 98, 99, 100}
	for _, perc := range percentages {
		if perc == 100 {
			fmt.Printf(" 100%%  %5.0f (longest request)\n",
				RoundMillisecond(durations["Total"][b.doneCount-1]))
		} else {
			fmt.Printf("  %d%%  %5.0f\n", perc,
				RoundMillisecond(durations["Total"][b.doneCount*perc/100]))
		}
	}
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
		if b.doneCount >= *requests && len(results) >= *requests {
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

func PrintUsage() {
	fmt.Println("Usage: go-ab [options] [http[s]://]hostname[:port]/path")
	fmt.Println("Options are:")
	flag.PrintDefaults()
}

func main() {
	verbosity = flag.Int("v", 0, "How much troubleshooting info to print")
	requests = flag.Int("n", 1, "Number of requests to perform")
	concurrency = flag.Int("c", 1, "Number of multiple requests to make at a time")
	keepalive = flag.Bool("k", false, "keep-alive connections")
	flag.Parse()

	rawurl := flag.Arg(0)
	if rawurl == "" {
		PrintUsage()
		return
	}

	err := ParseUrl(rawurl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: invalid URL\n", rawurl)
		PrintUsage()
		return
	}

	if *requests < *concurrency {
		fmt.Fprintf(os.Stderr, "%s: Cannot use concurrency level greater than total number of requests\n", os.Args[0])
		PrintUsage()
		return
	}

	log.SetPrefix("LOG: ")
	log.SetFlags(0)

	results = make(ResultList, 0, *concurrency)

	Test()
}
