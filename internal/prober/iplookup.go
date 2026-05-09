package prober

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type IPLookup struct {
	Endpoints []string
	Client    *http.Client
	Timeout   time.Duration
}

// DefaultIPLookupEndpoints — used when none are configured.
// All return the IP as plain text.
var DefaultIPLookupEndpoints = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://api.myip.com",
}

func (l *IPLookup) Get() (string, error) {
	var lastErr error
	for _, ep := range l.Endpoints {
		req, err := http.NewRequest("GET", ep, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := l.Client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("%s: HTTP %d", ep, resp.StatusCode)
			continue
		}
		ip := strings.TrimSpace(string(body))
		// api.myip.com returns JSON {"ip":"...",...}; handle that
		if strings.HasPrefix(ip, "{") {
			// crude extract — look for "ip":"..."
			if idx := strings.Index(ip, `"ip":"`); idx >= 0 {
				rest := ip[idx+6:]
				if end := strings.Index(rest, `"`); end > 0 {
					ip = rest[:end]
				}
			}
		}
		if ip == "" {
			lastErr = fmt.Errorf("%s: empty body", ep)
			continue
		}
		return ip, nil
	}
	if lastErr == nil {
		lastErr = errors.New("no endpoints configured")
	}
	return "", fmt.Errorf("all ip lookups failed: %w", lastErr)
}
