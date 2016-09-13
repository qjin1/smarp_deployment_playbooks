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
	reverseProxy := httputil.NewSingleHostReverseProxy(target)
	reverseProxy.FlushInterval = flushInterval
	m.ReverseProxy = reverseProxy
	return nil
}
func (m *ReverseProxyMarshal) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.url)
}

type ReverseProxyMarshal struct {
	ReverseProxy *httputil.ReverseProxy
	url          string
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
func getProxyFromRevision(revision string) *httputil.ReverseProxy {
	proxy := revisionProxyMap[revision]
	if proxy == nil {
		// get default revision
		proxy = revisionProxyMap[versionRevisionMap["stable"]]
	}
	return proxy.ReverseProxy
}
func getSubdomain(domain string) string {
	index := strings.Index(domain, ".")
	if index < 0 {
		index = 0
	}
	return domain[:index]
}

func shouldIUseThisProxy(r *http.Request, p *httputil.ReverseProxy) bool {
	if p == nil {
		return false
	}
	sniffReq := new(http.Request)
	*sniffReq = *r
	sniffReq.Method = "HEAD"
	p.Director(sniffReq)
	transport := p.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	sniffRes, err := transport.RoundTrip(sniffReq)
	if err == nil && sniffRes.StatusCode == http.StatusOK {
		return true
	}
	return false
}
func getProxyWithAvailbleAssets(r *http.Request) *httputil.ReverseProxy {
	for _, mp := range revisionProxyMap {
		p := mp.ReverseProxy
		if shouldIUseThisProxy(r, p) {
			return p
		}
	}
	return nil
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
			subdomain := getSubdomain(r.Host)
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
			subdomain := getSubdomain(r.Host)
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

	// @todo: we can move this down, so that if both designated proxy and default (stable) proxy are unavailable
	// we can use whatever left. However, we must have better ping/monitor before doing so, so that we don't have
	// silent failure
	if proxy == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Service unavailable!\n"))
		return

	}

	last6CharIndex := len(r.URL.Path) - 6
	if last6CharIndex < 0 {
		last6CharIndex = 0
	}
	// 1. It must be "GET" request
	// 2. It must be static asseet, i.e. last 6 character should have ".json" or sth like that
	// 3. Current Proxy is not available
	if r.Method == "GET" && strings.Contains(r.URL.Path[last6CharIndex:], ".") && !shouldIUseThisProxy(r, proxy) {
		if proxyWithAvailableAsset := getProxyWithAvailbleAssets(r); proxyWithAvailableAsset != nil {
			proxyWithAvailableAsset.ServeHTTP(w, r)
			return
		}
	}
	// serve whatever status code and body it is originally
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
		err := registerRevisionProxy(revision, proxyUrl)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
		}
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

func registerRevisionProxy(revision, proxyUrl string) error {
	proxy, err := newReverseProxyMarshal(proxyUrl)
	proxy.ReverseProxy.FlushInterval = flushInterval
	if err != nil {
		return err
	}
	revisionProxyMap[revision] = proxy
	return nil
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
	Main(os.Args[1], os.Args[2])
}

func Main(p1, p2 string) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		f, err := os.Open("data/vr.json")
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
		f, err := os.Open("data/sv.json")
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
		f, err := os.Open("data/rp.json")
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
		errChan <- http.ListenAndServe(":"+p1, proxyHandler)
	}()
	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/sv", subdomainVersionHandler)
		mux.HandleFunc("/vr", versionRevisionHandler)
		mux.HandleFunc("/rp", revisionProxyHandler)
		errChan <- http.ListenAndServe(":"+p2, mux)
	}()
	panic(<-errChan)
}
