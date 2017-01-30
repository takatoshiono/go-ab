package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
)

func handler(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "Hello, world\n")
}

func main() {
	port := flag.Int("p", 8000, "Port to listen")
	flag.Parse()
	http.HandleFunc("/", handler)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
