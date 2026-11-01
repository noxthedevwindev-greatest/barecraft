package main

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const appUA = "barecraft/1.0 (+https://github.com/noxthedevwindev-greatest/barecraft)"

var httpClient = &http.Client{
	// generous overall budget so big files can finish on slow links; the
	// fine-grained stall cases are bounded by the transport timeouts and the
	// watchdog reader instead
	Timeout: 30 * time.Minute,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 25 * time.Second,
		ExpectContinueTimeout: time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

// stallReader fails a download when no bytes arrive for d, so a dead
// connection errors out fast (and gets retried) instead of hanging forever.
type stallReader struct {
	mu    sync.Mutex
	r     io.ReadCloser
	d     time.Duration
	err   error
	done  chan struct{}
	reset chan struct{}
}

func newStallReader(r io.ReadCloser, d time.Duration) *stallReader {
	s := &stallReader{r: r, d: d, done: make(chan struct{}), reset: make(chan struct{}, 16)}
	go func() {
		t := time.NewTimer(d)
		defer t.Stop()
		for {
			select {
			case <-s.reset:
				if !t.Stop() {
					select {
					case <-t.C:
					default:
					}
				}
				t.Reset(d)
			case <-t.C:
				s.mu.Lock()
				s.err = fmt.Errorf("connection stalled (no data for %v)", d)
				s.mu.Unlock()
				return
			case <-s.done:
				return
			}
		}
	}()
	return s
}

func (s *stallReader) Read(p []byte) (int, error) {
	s.mu.Lock()
	e := s.err
	s.mu.Unlock()
	if e != nil {
		return 0, e
	}
	n, err := s.r.Read(p)
	if n > 0 {
		select {
		case s.reset <- struct{}{}:
		default:
		}
	}
	return n, err
}

func (s *stallReader) Close() error {
	close(s.done)
	return s.r.Close()
}

func httpGet(url string) (*http.Response, error) {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", appUA)
		resp, err := httpClient.Do(req)
		if err != nil {
			last = err
			continue
		}
		resp.Body = newStallReader(resp.Body, 60*time.Second)
		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return resp, nil // caller decides; no retry on 404
		}
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			last = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("GET %s failed after retries: %w", url, last)
}

func getBytes(url string) ([]byte, error) {
	resp, err := httpGet(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func getJSON(url string, v any) error {
	b, err := getBytes(url)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("parse %s: %w", url, err)
	}
	return nil
}

// downloadFile fetches url to dest atomically (.part then rename).
// Skips when dest already exists with the expected size.
func downloadFile(url, dest string, expectSize int64, sha1hex, sha256hex string) error {
	if st, err := os.Stat(dest); err == nil {
		if expectSize > 0 && st.Size() == expectSize {
			return nil
		}
		if expectSize == 0 && st.Size() > 0 && sha1hex == "" && sha256hex == "" {
			return nil
		}
		if st.Size() > 0 && expectSize == 0 {
			// no size known: trust it if hash (when given) checks out
			if sha1hex == "" && sha256hex == "" {
				return nil
			}
			if ok, _ := fileHashMatches(dest, sha1hex, sha256hex); ok {
				return nil
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		if err := fetchOnce(url, dest, expectSize, sha1hex, sha256hex); err != nil {
			if strings.Contains(err.Error(), "HTTP 404") {
				return err // permanent; fall through to next mirror immediately
			}
			last = err
			continue
		}
		return nil
	}
	return fmt.Errorf("download %s failed after retries: %w", url, last)
}

func fetchOnce(url, dest string, expectSize int64, sha1hex, sha256hex string) error {
	resp, err := httpGet(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	h1 := sha1.New()
	h256 := sha256.New()
	w := io.MultiWriter(f, h1, h256)
	n, err := io.Copy(w, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if expectSize > 0 && n != expectSize {
		os.Remove(tmp)
		return fmt.Errorf("size mismatch: got %d want %d", n, expectSize)
	}
	if sha1hex != "" && hex.EncodeToString(h1.Sum(nil)) != sha1hex {
		os.Remove(tmp)
		return fmt.Errorf("sha1 mismatch")
	}
	if sha256hex != "" && hex.EncodeToString(h256.Sum(nil)) != sha256hex {
		os.Remove(tmp)
		return fmt.Errorf("sha256 mismatch")
	}
	return os.Rename(tmp, dest)
}

func fileHashMatches(path, sha1hex, sha256hex string) (bool, error) {
	if sha1hex == "" && sha256hex == "" {
		return true, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h1 := sha1.New()
	h256 := sha256.New()
	if _, err := io.Copy(io.MultiWriter(h1, h256), f); err != nil {
		return false, err
	}
	if sha1hex != "" && hex.EncodeToString(h1.Sum(nil)) != sha1hex {
		return false, nil
	}
	if sha256hex != "" && hex.EncodeToString(h256.Sum(nil)) != sha256hex {
		return false, nil
	}
	return true, nil
}

// pool runs tasks with a concurrency cap, keeping the first error.
type pool struct {
	sem      chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
	firstErr error
}

func newPool(n int) *pool { return &pool{sem: make(chan struct{}, n)} }

func (p *pool) add(f func() error) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.sem <- struct{}{}
		defer func() { <-p.sem }()
		if err := f(); err != nil {
			p.mu.Lock()
			if p.firstErr == nil {
				p.firstErr = err
			}
			p.mu.Unlock()
		}
	}()
}

func (p *pool) wait() error {
	p.wg.Wait()
	return p.firstErr
}
