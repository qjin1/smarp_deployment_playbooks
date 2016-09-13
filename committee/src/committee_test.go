package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTimeConsuming(t *testing.T) {
	go Main("2000", "2001")

	var backendServer900 *httptest.Server
	var backendServer901 *httptest.Server

	{
		mux := http.NewServeMux()
		mux.HandleFunc("/900", func(w http.ResponseWriter, r *http.Request) {
			//				time.Sleep(200 *time.Millisecond)
			w.Write([]byte("9000 stable"))
		})
		backendServer900 = httptest.NewServer(mux)
		defer backendServer900.Close()
	}
	{
		mux := http.NewServeMux()
		mux.HandleFunc("/901", func(w http.ResponseWriter, r *http.Request) {
			//				time.Sleep(200 *time.Millisecond)
			w.Write([]byte("9001 unstable"))
		})
		backendServer901 = httptest.NewServer(mux)
		defer backendServer901.Close()
	}

	versionRevisionMap = map[string]string{"stable": "900", "901": "901"}
	registerRevisionProxy("900", backendServer900.URL)
	registerRevisionProxy("901", backendServer901.URL)

	resp, err := http.Get("http://localhost:2000/900")
	if err != nil {
		t.Fatal(err)
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Status code not 200 but %v", resp.StatusCode)
	}

	resp, err = http.Get("http://localhost:2000/901")
	if err != nil {
		t.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Fatalf("Status code not 200 but %v", resp.StatusCode)
	}

	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
}
