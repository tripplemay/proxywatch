package prober

import (
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

// Result is the output of a single probe run.
type Result struct {
	TS        time.Time
	Target    string
	HTTPCode  int
	LatencyMS int
	ExitIP    string
	OK        bool // 200 ≤ code < 400 AND no transport error
	RawError  string
}

type ActiveProber struct {
	Target   string
	Timeout  time.Duration
	Client   *http.Client
	IPLookup func() (string, error)
}

func (p *ActiveProber) Run() Result {
	start := time.Now()
	r := Result{TS: start, Target: p.Target}

	req, err := http.NewRequest("GET", p.Target, nil)
	if err != nil {
		r.RawError = err.Error()
		return r
	}
	resp, err := p.Client.Do(req)
	r.LatencyMS = int(time.Since(start).Milliseconds())
	if err != nil {
		r.RawError = err.Error()
	} else {
		resp.Body.Close()
		r.HTTPCode = resp.StatusCode
		r.OK = resp.StatusCode >= 200 && resp.StatusCode < 400
	}

	if p.IPLookup != nil {
		ip, ipErr := p.IPLookup()
		if ipErr == nil {
			r.ExitIP = ip
		}
	}
	return r
}

// NewSOCKS5Client builds an http.Client whose transport routes through the SOCKS5 URL.
func NewSOCKS5Client(socksURL string, timeout time.Duration) (*http.Client, error) {
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
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Dial: dialer.Dial,
		},
	}, nil
}
