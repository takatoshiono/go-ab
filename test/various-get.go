package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

type Benchmark struct {
	url string
	n   int
	c   int
}

// https://golang.org/pkg/net/http/#Get
// ex) 100 requests, 4 concurrent, use 20 ports
// http.Get is wrapper around DefaultClient.Get(url)
// https://github.com/golang/go/blob/753452fac6f6963b5a6e38a239b05362385a3842/src/net/http/client.go#L413-L421
// http.Get calls NewRequest every time.
func (b *Benchmark) Get(n int) {
	for i := 0; i < n; i++ {
		resp, err := http.Get(b.url)
		if err != nil {
			log.Fatal("GET %s failed: %+v\n", b.url, err)
		}
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}
}

// re-use request
// ex) 100 requests, 4 concurrent, use 18 ports
// 変わらないじゃん
// これかな...
// https://golang.org/pkg/net/http/#RoundTripper
// DefaultTransport is the default implementation of Transport and is used by DefaultClient. It establishes network connections as needed and caches them for reuse by subsequent calls.
// あとはこの辺
// https://golang.org/pkg/net/http/#Transport
// By default, Transport caches connections for future re-use. This may leave many open connections when accessing many hosts. This behavior can be managed using Transport's CloseIdleConnections method and the MaxIdleConnsPerHost and DisableKeepAlives fields.
func (b *Benchmark) Get2(n int) {
	req, err := http.NewRequest("GET", b.url, nil)
	if err != nil {
		log.Fatal("NewRequest failed: %+v\n", err)
		return
	}

	for i := 0; i < n; i++ {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatal("GET %s failed: %+v\n", b.url, err)
		}
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}
}

// Use DefaultTransport explictly
func (b *Benchmark) Get3(n int) {
	client := &http.Client{Transport: http.DefaultTransport}

	req, err := http.NewRequest("GET", b.url, nil)
	if err != nil {
		log.Fatal("NewRequest failed: %+v\n", err)
		return
	}

	for i := 0; i < n; i++ {
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal("GET %s failed: %+v\n", b.url, err)
		}
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}
}

// Use my Transport
func (b *Benchmark) Get4(n int) {
	// あれー？これでDefaultTransportと同じはずだけど、concurrencyぶんのportしか使われないぞ
	// goroutineごとにtransportを作るか、共通のtransportを使うかの違いかな
	var tr http.RoundTripper = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	client := &http.Client{Transport: tr}

	req, err := http.NewRequest("GET", b.url, nil)
	if err != nil {
		log.Fatal("NewRequest failed: %+v\n", err)
		return
	}

	for i := 0; i < n; i++ {
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal("GET %s failed: %+v\n", b.url, err)
		}
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}
}

var MyTransport http.RoundTripper = &http.Transport{}

// Use common transport
func (b *Benchmark) Get5(n int) {
	client := &http.Client{Transport: MyTransport}

	req, err := http.NewRequest("GET", b.url, nil)
	if err != nil {
		log.Fatal("NewRequest failed: %+v\n", err)
		return
	}

	for i := 0; i < n; i++ {
		resp, err := client.Do(req)
		if err != nil {
			log.Fatal("GET %s failed: %+v\n", b.url, err)
		}
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}
}

func (b *Benchmark) Run() {
	var wg sync.WaitGroup
	wg.Add(b.c)

	n := b.n / b.c
	for i := 0; i < b.c; i++ {
		go func() {
			//b.Get(n)
			//b.Get2(n)
			//b.Get3(n)
			//b.Get4(n)
			b.Get5(n)
			wg.Done()
		}()
	}
	wg.Wait()
}

func main() {
	var requests *int
	var concurrency *int

	requests = flag.Int("n", 1, "Number of requests to perform")
	concurrency = flag.Int("c", 1, "Number of multiple requests to make at a time")
	flag.Parse()

	url := flag.Arg(0)
	if url == "" {
		fmt.Println("Usage: go-ab2 [options] [http[s]://]hostname[:port]/path")
		return
	}

	b := &Benchmark{
		url: url,
		n:   *requests,
		c:   *concurrency,
	}
	b.Run()

	fmt.Printf("%d requests done.\n", *requests)
}
