package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type httpTracker struct {
	t        http.RoundTripper
	l        sync.Mutex
	inflight map[string]time.Time
}

func (t *httpTracker) register(u string) {
	t.l.Lock()
	defer t.l.Unlock()
	t.inflight[u] = time.Now()
}

func (t *httpTracker) unregister(u string) {
	t.l.Lock()
	defer t.l.Unlock()
	delete(t.inflight, u)
}

func (t *httpTracker) reportOnce() {
	t.l.Lock()
	defer t.l.Unlock()

	log.Printf("In-flight HTTP requests:")
	for k, t := range t.inflight {
		log.Printf("  servicing %q for %v", k, time.Since(t))
	}
}

func (t *httpTracker) report(ch <-chan os.Signal) {
	for _ = range ch {
		t.reportOnce()
	}
}

type trackFinalizer struct {
	b io.ReadCloser
	t *httpTracker
	u string
}

func (d *trackFinalizer) Close() error {
	d.t.unregister(d.u)
	return d.b.Close()
}

func (d *trackFinalizer) Read(b []byte) (int, error) {
	return d.b.Read(b)
}

func (d *trackFinalizer) WriteTo(w io.Writer) (n int64, err error) {
	return io.Copy(w, d.b)
}

func (t *httpTracker) RoundTrip(req *http.Request) (*http.Response, error) {
	u := req.URL.String()
	t.register(u)
	res, err := t.t.RoundTrip(req)
	res.Body = &trackFinalizer{res.Body, t, u}
	return res, err
}

func initHttpMagic() {
	http.DefaultTransport = &httpTracker{
		t:        http.DefaultTransport,
		inflight: map[string]time.Time{},
	}

	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, syscall.Signal(29))

	go http.DefaultTransport.(*httpTracker).report(sigch)
}
