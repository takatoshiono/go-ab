package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
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
var host string
var port int
var path string
var doclen int

type Benchmark struct {
	start        time.Time
	lasttime     time.Time
	startedCount int
	doneCount    int
	mux          sync.Mutex
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

func (b *Benchmark) TimeTaken() float64 {
	return b.lasttime.Sub(b.start).Seconds()
}

func (b *Benchmark) RequestPerSecond() float64 {
	return float64(b.doneCount) / b.TimeTaken()
}

var b = &Benchmark{}

type ConnectionTimes struct {
	start     time.Time // Start of connection
	connect   time.Time // Connected start writing
	endwrite  time.Time // Request written
	beginread time.Time // First byte of input
	done      time.Time // Connection closed
}

func Request(c chan string) {
	for requestUrl := range c {
		if b.startedCount >= *requests {
			continue
		}
		b.IncrStarted()

		connTimes := &ConnectionTimes{}

		trace := &httptrace.ClientTrace{
			ConnectStart: func(network, addr string) {
				connTimes.start = time.Now()
				b.SetLasttime(time.Now())
				//fmt.Println("ConnectStart:", connTimes.start, network, addr)
			},
			GotConn: func(info httptrace.GotConnInfo) {
				connTimes.connect = time.Now()
				b.SetLasttime(time.Now())
				//fmt.Printf("GotConn: %v %+v\n", connTimes.connect, info)
			},
			WroteRequest: func(info httptrace.WroteRequestInfo) {
				if info.Err != nil {
					fmt.Println("Failed to write the request", info.Err)
				}
				connTimes.endwrite = time.Now()
				b.SetLasttime(time.Now())
				//fmt.Println("WroteRequest:", connTimes.endwrite)
			},
		}

		req, err := http.NewRequest("GET", requestUrl, nil)
		if err != nil {
			fmt.Println(err)
			return
		}

		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer resp.Body.Close()
		connTimes.beginread = time.Now()
		b.SetLasttime(time.Now())

		// TODO: read headers and body
		LogDebugf("Response code = %s\n", resp.Status)

		servername = resp.Header.Get("Server")
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println(err)
			return
		}
		doclen = len(body)

		b.IncrDone()
		connTimes.done = time.Now()
		b.SetLasttime(time.Now())
	}
}

func OutputResults() {
	fmt.Printf("\n\n")
	fmt.Printf("Server Software:        %s\n", servername)
	fmt.Printf("Server Hostname:        %s\n", host)
	fmt.Printf("Server Port:            %d\n", port)
	fmt.Printf("\n")
	fmt.Printf("Document Path:          %s\n", path)
	fmt.Printf("Document Length:        %d bytes\n", doclen)
	fmt.Printf("Time taken for tests:   %.3f seconds\n", b.TimeTaken())
	fmt.Printf("Complete requests:      %d\n", b.doneCount)
	fmt.Printf("Requests per second:    %.2f [#/sec] (mean)\n", b.RequestPerSecond())
}

func Test() {
	fmt.Printf("Benchmarking %s ", host)
	fmt.Printf("(be patient)%s", "...")

	b.start = time.Now()
	b.lasttime = time.Now()

	ch := make([]chan string, *concurrency)
	for i := 0; i < *concurrency; i++ {
		ch[i] = make(chan string)
		go Request(ch[i])
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
	host = h[0]
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

	Test()
}
