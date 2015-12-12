package main

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

func entryPoint(path string) bool {
	//	var entryPointMap = map[string]struct{}{
	//		"/": struct{}{},
	//	}
	if path == "/" {
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
const (
	versionKey   string = "version"
	revisionKey         = "revision"
	subdomainKey        = "subdomain"
	proxyKey            = "p"
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
func getProxyFromRevision(revision string) http.Handler {
	proxy := revisionProxyMap[revision]
	if proxy == nil {
		// get default revision
		proxy = revisionProxyMap[versionRevisionMap["stable"]]
		if proxy == nil {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("Service unavailable!"))
			})
			return handler
		}
	}
	return proxy
}

var proxyHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	var revision string
	path := r.URL.Path
	if path == "/api/lb/subdomain" {
		subdomainVersionHandler(w, r)
		return
	}
	if path == "/api/lb/version" {
		versionRevisionHandler(w, r)
		return
	}
	if entryPoint(path) {
		var versionServed string
		// if the request is an entry point
		// check version from param, then cookie, then subdomain, then fallback to stable
		// 1. param
		versionAsked := r.URL.Query().Get(versionKey)
		if versionAsked == "" {
			// 2. cookie v
			c, _ := r.Cookie(versionKey)
			if c != nil {
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
				Name:  versionKey,
				Value: versionAsked,
			})
		}
		revision = getRevisionFromVersion(versionServed)

		http.SetCookie(w, &http.Cookie{
			Name:  revisionKey,
			Value: revision,
		})
	} else {
		// check revision from param, then cookie, then version from subdomain, then fallback to stable
		// 1. param
		revision = r.URL.Query().Get(revisionKey)
		if revision == "" {
			// 2. cookie
			if c, err := r.Cookie(revisionKey); err == nil {
				revision = c.Value
			}
		}
		if revision == "" {
			domain := r.Host
			// @todo unsafe
			subdomain := domain[:strings.Index(domain, ".")]
			// 3. subdomain
			versionServed := getVersionFromSubdomain(subdomain)
			revision = getRevisionFromVersion(versionServed)
		}
		if revision == "" {
			// 4. stable
			revision = getRevisionFromVersion("stable")
		}
	}
	proxy := getProxyFromRevision(revision)
	proxy.ServeHTTP(w, r)
}
var subdomainVersionHandler = func(w http.ResponseWriter, r *http.Request) {
	subdomain := r.FormValue(subdomainKey)
	if subdomain == "" {
		subdomain = r.FormValue(subdomainKey)
	}
	switch r.Method {
	case "GET":
		e := json.NewEncoder(w)
		if subdomain != "" {
			e.Encode(map[string]string{
				subdomain: subdomainVersionMap[subdomain],
			})
			return
		}
		e.Encode(subdomainVersionMap)
	case "POST":
		if subdomain == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		version := r.FormValue(versionKey)
		//		if version == "" {
		//			version = r.FormValue("version")
		//		}
		if version == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		subdomainVersionMap[subdomain] = version
		w.WriteHeader(http.StatusNoContent)
	case "DELETE":
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
		revision := r.FormValue(revisionKey)
		if revision == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		proxyUrl := r.PostFormValue(proxyKey)
		proxy, err := newReverseProxyMarshal(proxyUrl)
		proxy.FlushInterval = flushInterval
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		revisionProxyMap[revision] = proxy
		w.WriteHeader(http.StatusNoContent)
	case "DELETE":
		revision := r.FormValue(revisionKey)
		if revision == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		delete(revisionProxyMap, revision)
		w.WriteHeader(http.StatusNoContent)
	}
}

var versionRevisionHandler = func(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		e := json.NewEncoder(w)
		e.Encode(versionRevisionMap)
	case "POST":
		version := r.FormValue(versionKey)
		if version == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		revision := r.FormValue(revisionKey)
		if revision == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		versionRevisionMap[version] = revision
		w.WriteHeader(http.StatusNoContent)
	case "DELETE":
		version := r.FormValue(versionKey)
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
		mux.HandleFunc("/sv", subdomainVersionHandler)
		mux.HandleFunc("/vr", versionRevisionHandler)
		mux.HandleFunc("/rp", revisionProxyHandler)
		errChan <- http.ListenAndServe(":"+os.Args[2], mux)
	}()
	panic(<-errChan)
}
