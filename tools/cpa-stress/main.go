package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "0.1.0-dev"

func main() {
	var (
		apiKey    string
		baseURL   string
		socksURL  string
		outputDir string
		dryRun    bool
		showVer   bool
	)
	flag.StringVar(&apiKey, "api-key", "", "CPA client API key (required)")
	flag.StringVar(&baseURL, "base-url", "https://api.vpanel.cc", "CPA base URL")
	flag.StringVar(&socksURL, "socks-url", "", "SOCKS5 URL for exit-IP sampling, e.g. socks5h://user:pass@host:port (required)")
	flag.StringVar(&outputDir, "output-dir", ".", "where to write run-<ts>.jsonl and report dir")
	flag.BoolVar(&dryRun, "dry-run", false, "short test (each step 30s, max C=4)")
	flag.BoolVar(&showVer, "version", false, "print version and exit")
	flag.Parse()

	if showVer {
		fmt.Println("cpa-stress", version)
		return
	}
	if apiKey == "" || socksURL == "" {
		fmt.Fprintln(os.Stderr, "error: -api-key and -socks-url are required")
		fmt.Fprintln(os.Stderr, "       -socks-url example: socks5h://user:pass@us.miyaip.online:1111")
		os.Exit(2)
	}

	fmt.Println("cpa-stress", version, "(skeleton — orchestration added in Task 6.1)")
	_ = baseURL
	_ = outputDir
	_ = dryRun
}
