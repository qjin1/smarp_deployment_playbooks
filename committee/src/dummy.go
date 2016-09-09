package main

import (
//	"time"
	"net/http"
)

func main() {

	errChan := make(chan error, 1)
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
//				time.Sleep(200 *time.Millisecond)
				w.Write([]byte("9001 unstable"))
			})
		errChan <- http.ListenAndServe(":9001", mux)
	}()
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
//				time.Sleep(200 *time.Millisecond)
				w.Write([]byte("9000 stable"))
			})
		errChan <- http.ListenAndServe(":9000", mux)
	}()
	print("running!")
	panic(<-errChan)
}
