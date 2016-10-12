package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"time"
)

var requests *int
var url string

var start time.Time
var lasttime time.Time
var done int

type ConnectionTimes struct {
	start     time.Time // Start of connection
	connect   time.Time // Connected start writing
	endwrite  time.Time // Request written
	beginread time.Time // First byte of input
	done      time.Time // Connection closed
}

func Request(c chan string) {
	for url := range c {
		connTimes := &ConnectionTimes{}

		trace := &httptrace.ClientTrace{
			ConnectStart: func(network, addr string) {
				connTimes.start = time.Now()
				lasttime = time.Now()
				fmt.Println("ConnectStart:", connTimes.start, network, addr)
			},
			GotConn: func(info httptrace.GotConnInfo) {
				connTimes.connect = time.Now()
				lasttime = time.Now()
				fmt.Printf("GotConn: %v %+v\n", connTimes.connect, info)
			},
			WroteRequest: func(info httptrace.WroteRequestInfo) {
				if info.Err != nil {
					fmt.Println("Failed to write the request", info.Err)
				}
				connTimes.endwrite = time.Now()
				lasttime = time.Now()
				fmt.Println("WroteRequest:", connTimes.endwrite)
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
		done++
		connTimes.done = time.Now()
		lasttime = time.Now()
		fmt.Println(resp.Status)
	}
}

func OutputResults() {
	timeTaken := lasttime.Sub(start)
	fmt.Printf("Time taken for tests:   %.3f seconds\n", timeTaken.Seconds())
	fmt.Printf("Requests per second:    %.2f [#/sec] (mean)\n", float64(done)/timeTaken.Seconds())
}

func Test() {
	start = time.Now()
	lasttime = time.Now()

	ch := make(chan string)
	go Request(ch)

	for {
		ch <- url
		if done >= *requests {
			break
		}
	}

	fmt.Printf("Finished %d requests\n", done)
	OutputResults()
}

func main() {
	requests = flag.Int("n", 1, "Number of requests")
	flag.Parse()

	url = flag.Arg(0)
	if url == "" {
		fmt.Println("Usage: go-ab [http[s]://]hostname[:port]/path")
		return
	}

	Test()
}
