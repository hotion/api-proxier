package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type response struct {
	Msg  string `json:"data"`
	Code int    `json:"code"`
}

func runServer1() {

	mux := http.NewServeMux()

	mux.HandleFunc("/srv/name", func(w http.ResponseWriter, req *http.Request) {
		req.ParseMultipartForm(32 << 10)
		name := req.Form.Get("name")
		msg := name
		if name == "" {
			msg = "error, empty name value"
		}
		bs, _ := json.Marshal(response{Msg: msg, Code: 0})
		fmt.Fprintf(w, string(bs))
		return
	})

	mux.HandleFunc("/srv/id", func(w http.ResponseWriter, req *http.Request) {
		req.ParseForm()

		id := req.Form.Get("id")
		msg := id
		if id == "" {
			msg = "error, empty id"
		}

		bs, _ := json.Marshal(response{Msg: msg, Code: 0})
		fmt.Fprintf(w, string(bs))
		return
	})

	srv := &http.Server{
		Handler: mux,
		Addr:    ":9091",
	}
	log.Printf("listen on: %s\n", ":9091")
	log.Fatal(srv.ListenAndServe())
}

func runServer2() {

	mux := http.NewServeMux()

	mux.HandleFunc("/srv/name", func(w http.ResponseWriter, req *http.Request) {
		bs, _ := json.Marshal(response{Msg: "server2", Code: 0})
		fmt.Fprintf(w, string(bs))
		return
	})

	mux.HandleFunc("/srv/id", func(w http.ResponseWriter, req *http.Request) {
		bs, _ := json.Marshal(response{Msg: "2", Code: 0})
		fmt.Fprintf(w, string(bs))
		return
	})

	// http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
	// 	bs, _ := json.Marshal(response{Msg: "this is server 2", Code: 0})
	// 	fmt.Fprintf(w, string(bs))
	// 	return
	// })

	srv := &http.Server{
		Handler: mux,
		Addr:    ":9092",
	}

	log.Printf("listen on: %s\n", ":9092")
	log.Fatal(srv.ListenAndServe())
}

func main() {
	go runServer1()
	go runServer2()

	quit := make(chan bool)
	<-quit
}
