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

func runServer(addr string, name string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/srv/name", func(w http.ResponseWriter, req *http.Request) {
		req.ParseForm()
		var msg string

		if req.Form.Get("name") == "" {
			msg = name + "error, empty name value"
		} else {
			msg = name
		}
		bs, _ := json.Marshal(response{Msg: msg, Code: 0})
		fmt.Fprintf(w, string(bs))
		return
	})

	mux.HandleFunc("/srv/id", func(w http.ResponseWriter, req *http.Request) {
		req.ParseForm()
		bs, _ := json.Marshal(response{Msg: name, Code: 0})
		fmt.Fprintf(w, string(bs))
		return
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, req *http.Request) {
		bs, _ := json.Marshal(response{Msg: "ok", Code: 0})
		fmt.Fprintf(w, string(bs))
		return
	})

	srv := &http.Server{
		Handler: mux,
		Addr:    addr,
	}
	log.Printf("listen on: %s\n", addr)
	log.Fatal(srv.ListenAndServe())
}

func main() {
	go runServer(":9091", "srv1")
	go runServer(":9092", "srv2")
	go runServer(":9093", "srv3")

	quit := make(chan bool)
	<-quit
}
