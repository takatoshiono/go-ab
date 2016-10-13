package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

var requests *int
var concurrency *int
var url string

type Done struct {
	count int
	mux   sync.Mutex
}

func (d *Done) Increment() {
	d.mux.Lock()
	defer d.mux.Unlock()
	d.count++
}

type Started struct {
	count int
	mux   sync.Mutex
}

func (s *Started) Increment() {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.count++
}

var start time.Time
var lasttime time.Time
var started = &Started{}
var done = &Done{}

type ConnectionTimes struct {
	start     time.Time // Start of connection
	connect   time.Time // Connected start writing
	endwrite  time.Time // Request written
	beginread time.Time // First byte of input
	done      time.Time // Connection closed
}

func Request(c chan string) {
	for url := range c {
		if started.count >= *requests {
			continue
		}
		started.Increment()

		connTimes := &ConnectionTimes{}

		trace := &httptrace.ClientTrace{
			ConnectStart: func(network, addr string) {
				connTimes.start = time.Now()
				lasttime = time.Now()
				//fmt.Println("ConnectStart:", connTimes.start, network, addr)
			},
			GotConn: func(info httptrace.GotConnInfo) {
				connTimes.connect = time.Now()
				lasttime = time.Now()
				//fmt.Printf("GotConn: %v %+v\n", connTimes.connect, info)
			},
			WroteRequest: func(info httptrace.WroteRequestInfo) {
				if info.Err != nil {
					fmt.Println("Failed to write the request", info.Err)
				}
				connTimes.endwrite = time.Now()
				lasttime = time.Now()
				//fmt.Println("WroteRequest:", connTimes.endwrite)
			},
		}

		req, err := http.NewRequest("GET", url, nil)
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
		lasttime = time.Now()

		// TODO: read headers and body
		log.Printf("Response code = %s\n", resp.Status)

		done.Increment()
		connTimes.done = time.Now()
		lasttime = time.Now()
	}
}

func OutputResults() {
	timeTaken := lasttime.Sub(start)
	fmt.Printf("Time taken for tests:   %.3f seconds\n", timeTaken.Seconds())
	fmt.Printf("Complete requests:      %d\n", done.count)
	fmt.Printf("Requests per second:    %.2f [#/sec] (mean)\n", float64(done.count)/timeTaken.Seconds())
}

func Test() {
	fmt.Printf("Benchmarking...")

	start = time.Now()
	lasttime = time.Now()

	ch := make([]chan string, *concurrency)
	for i := 0; i < *concurrency; i++ {
		ch[i] = make(chan string)
		go Request(ch[i])
	}

	for {
		for i := 0; i < *concurrency; i++ {
			ch[i] <- url
		}
		if done.count >= *requests {
			break
		}
	}

	//fmt.Printf("Finished %d requests\n", done)
	fmt.Printf("..done\n")
	OutputResults()
}

func main() {
	requests = flag.Int("n", 1, "Number of requests to perform")
	concurrency = flag.Int("c", 1, "Number of multiple requests to make at a time")
	flag.Parse()

	url = flag.Arg(0)
	if url == "" {
		fmt.Println("Usage: go-ab [http[s]://]hostname[:port]/path")
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
