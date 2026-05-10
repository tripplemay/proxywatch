package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

// Sample is one captured exit IP at a moment in time.
type Sample struct {
	TSMS int64
	IP   string
}

// Sampler periodically queries an "ip lookup" endpoint and stores the latest result.
type Sampler struct {
	Endpoints []string
	HTTP      *http.Client
	Timeout   time.Duration

	mu     sync.RWMutex
	latest Sample
}

// DefaultIPLookupEndpoints — lookups are tried in order until one succeeds.
var DefaultIPLookupEndpoints = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
}

// NewSamplerOverSOCKS5 builds a Sampler whose HTTP client routes through the SOCKS5 URL.
func NewSamplerOverSOCKS5(socksURL string, timeout time.Duration) (*Sampler, error) {
	u, err := url.Parse(socksURL)
	if err != nil {
		return nil, err
	}
	var auth *proxy.Auth
	if u.User != nil {
		pw, _ := u.User.Password()
		auth = &proxy.Auth{User: u.User.Username(), Password: pw}
	}
	dialer, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
	if err != nil {
		return nil, err
	}
	hc := &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{Dial: dialer.Dial},
	}
	return &Sampler{
		Endpoints: DefaultIPLookupEndpoints,
		HTTP:      hc,
		Timeout:   timeout,
	}, nil
}

// Run polls every interval until ctx is cancelled.
func (s *Sampler) Run(ctx context.Context, interval time.Duration) {
	for {
		s.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (s *Sampler) tick(ctx context.Context) {
	for _, ep := range s.Endpoints {
		req, err := http.NewRequestWithContext(ctx, "GET", ep, nil)
		if err != nil {
			continue
		}
		resp, err := s.HTTP.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			continue
		}
		ip := strings.TrimSpace(string(body))
		if ip == "" {
			continue
		}
		s.mu.Lock()
		s.latest = Sample{TSMS: time.Now().UnixMilli(), IP: ip}
		s.mu.Unlock()
		return
	}
}

// Latest returns the most recent successful sample (zero Sample if none yet).
func (s *Sampler) Latest() Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latest
}
