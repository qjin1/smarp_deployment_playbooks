package main

import (
	"net/http/httputil"
	"net/http"
	"strings"
	"encoding/json"
	"os"
	"net/url"
	"sync"
	"time"
)

func entryPoint(path string) bool {
	//	var entryPointMap = map[string]struct{}{
	//		"/": struct{}{},
	//	}
	if (path == "/") {
		return true
	}
	return false
}

type Handler interface {
json.Marshaler
http.Handler
}
type marshallableProxy map[string]*ReverseProxyMarshal

const (
	flushInterval = 100 * time.Millisecond
)
func (m *ReverseProxyMarshal) UnmarshalJSON(inp []byte) (err error) {
	err = json.Unmarshal(inp, &m.url)
	if err != nil {
		return
	}
	target, err := url.Parse(m.url)
	if err != nil {
		return
	}
	m.ReverseProxy = httputil.NewSingleHostReverseProxy(target)
	m.FlushInterval = flushInterval
	return nil
}
func (m *ReverseProxyMarshal) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.url)
}

type ReverseProxyMarshal struct {
	*httputil.ReverseProxy
	url string
}

func newReverseProxyMarshal(u string) (r *ReverseProxyMarshal, err error) {
	target, err := url.Parse(u)
	if err != nil {
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	r = &ReverseProxyMarshal{
		proxy,
		u,
	}
	return r, nil
}

var subdomainVersionMap = map[string]string{}
var versionRevisionMap = map[string]string{}
var revisionProxyMap = marshallableProxy{}

func getVersionFromSubdomain(subdomain string) (version string) {
	version = subdomainVersionMap[subdomain]
	//	if version == "" {
	//		version = "stable"
	//	}
	return version
}
func getRevisionFromVersion(version string) (revision string) {
	revision = versionRevisionMap[version]
	if revision == "" {
		revision = versionRevisionMap["stable"]
	}
	return revision
}
func getProxyFromRevision(revision string) (http.Handler) {
	proxy := revisionProxyMap[revision]
	if proxy == nil {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Service unavailable!"))
		})
		return handler
	}
	return proxy
}

var proxyHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	var revision string
	if entryPoint(r.URL.Path) {
		var versionServed string
		// if the request is an entry point
		// check version from param, then cookie, then subdomain, then fallback to stable
		// 1. param
		versionAsked := r.FormValue("v")
		if versionAsked == "" {
			// 2. cookie
			if c, _ := r.Cookie("v"); c != nil {
				versionAsked = c.Value
			}
		}
		if versionAsked == "" {
			domain := r.Host
			// @todo unsafe
			subdomain := domain[:strings.Index(domain, ".")]
			// 3. subdomain
			versionServed = getVersionFromSubdomain(subdomain)
		} else {
			versionServed = versionAsked
			// if explicitly asked, save the version
			http.SetCookie(w, &http.Cookie{
				Name: "v",
				Value: versionAsked,
			})
		}
		revision = getRevisionFromVersion(versionServed)

		http.SetCookie(w, &http.Cookie{
			Name: "r",
			Value: revision,
		})
	} else {
		// check version from param, then cookie, then fallback to stable
		// #1. param
		revision = r.FormValue("r")
		if revision == "" {
			// #2. cookie
			if c, err := r.Cookie("r"); err == nil {
				revision = c.Value
			}
		}
		if revision == "" {
			// #3. stable
			revision = getRevisionFromVersion("stable")
		}
	}
	w.Header().Set("Expires", "-1")
	w.Header().Set("Cache-Control", "must-revalidate, private")
	proxy := getProxyFromRevision(revision)
	proxy.ServeHTTP(w, r)
}
var versionSubdomainHandler = func(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		e := json.NewEncoder(w)
		e.Encode(subdomainVersionMap)
	case "POST":
		version := r.FormValue("v")
		if version == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		subdomain := r.FormValue("s")
		if subdomain == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		subdomainVersionMap[subdomain] = version
		w.WriteHeader(http.StatusNoContent)
	case "DELETE":
		subdomain := r.FormValue("s")
		if subdomain == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		delete(subdomainVersionMap, subdomain)
		w.WriteHeader(http.StatusNoContent)
	}
}
var revisionProxyHandler = func(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		e := json.NewEncoder(w)
		e.Encode(revisionProxyMap)
	case "POST":
		revision := r.FormValue("r")
		if revision == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		proxyUrl := r.PostFormValue("p")
		proxy, err := newReverseProxyMarshal(proxyUrl)
		proxy.FlushInterval = flushInterval
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		revisionProxyMap[revision] = proxy
		w.WriteHeader(http.StatusNoContent)
	case "DELETE":
		revision := r.FormValue("r")
		if revision == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		delete(revisionProxyMap, revision)
		w.WriteHeader(http.StatusNoContent)
	}
}

var revisionVersionHandler = func(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		e := json.NewEncoder(w)
		e.Encode(versionRevisionMap)
	case "POST":
		version := r.FormValue("v")
		if version == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		revision := r.FormValue("r")
		if revision == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		versionRevisionMap[version] = revision
		w.WriteHeader(http.StatusNoContent)
	case "DELETE":
		version := r.FormValue("v")
		if version == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		delete(versionRevisionMap, version)
		w.WriteHeader(http.StatusNoContent)
	}
}

func main() {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		f, err := os.Open("vr.json")
		if err != nil {
			panic(err)
		}
		err = json.NewDecoder(f).Decode(&versionRevisionMap)
		if err != nil {
			panic(err)
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		f, err := os.Open("sv.json")
		if err != nil {
			panic(err)
		}
		err = json.NewDecoder(f).Decode(&subdomainVersionMap)
		if err != nil {
			panic(err)
		}
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		f, err := os.Open("rp.json")
		if err != nil {
			panic(err)
		}
		err = json.NewDecoder(f).Decode(&revisionProxyMap)
		if err != nil {
			panic(err)
		}
		wg.Done()
	}()
	wg.Wait()

	errChan := make(chan error, 1)
	go func() {
		errChan <- http.ListenAndServe(":"+os.Args[1], proxyHandler)
	}()
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/sv", versionSubdomainHandler)
		mux.HandleFunc("/vr", revisionVersionHandler)
		mux.HandleFunc("/rp", revisionProxyHandler)
		errChan <- http.ListenAndServe(":"+os.Args[2], mux)
	}()
	panic(<-errChan)
}
