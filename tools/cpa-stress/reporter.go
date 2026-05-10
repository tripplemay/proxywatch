package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// Report holds rows + metadata produced by a run.
type Report struct {
	StartTime     time.Time
	EndTime       time.Time
	StoppedReason StopReason
	Rows          []Row
}

// LoadReport reads a JSONL file written by Writer.
func LoadReport(path string) (*Report, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := &Report{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		var row Row
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			continue
		}
		r.Rows = append(r.Rows, row)
	}
	return r, sc.Err()
}

type stepStat struct {
	step        int
	concurrency int
	count       int
	ok          int
	c4xx        int
	c5xx        int
	terr        int
	startMS     int64
	endMS       int64
	latencies   []int
	tokIn       int
	tokOut      int
}

type modelStat struct {
	model      string
	count      int
	ok         int
	c4xx       int
	totLatency int64
	tokIn      int
	tokOut     int
}

type ipStat struct {
	ip        string
	count     int
	ok        int
	c4xx      int
	firstStep int
	lastStep  int
}

func percentile(latencies []int, p float64) int {
	if len(latencies) == 0 {
		return 0
	}
	cp := append([]int(nil), latencies...)
	sort.Ints(cp)
	idx := int(float64(len(cp)) * p)
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}

// WriteMarkdown serializes the report to a markdown file.
func (rep *Report) WriteMarkdown(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()

	now := time.Now()
	if rep.EndTime.IsZero() {
		rep.EndTime = now
	}
	dur := rep.EndTime.Sub(rep.StartTime)
	totalIn, totalOut := 0, 0
	for _, r := range rep.Rows {
		totalIn += r.InTokens
		totalOut += r.OutTokens
	}

	fmt.Fprintf(w, "# CPA Stress Test Report — %s\n\n", rep.EndTime.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "## Summary\n")
	fmt.Fprintf(w, "- Total duration: %s\n", dur.Round(time.Second))
	fmt.Fprintf(w, "- Total requests: %d\n", len(rep.Rows))
	fmt.Fprintf(w, "- Stopped reason: `%s`\n", rep.StoppedReason)
	fmt.Fprintf(w, "- Total input tokens: %d\n", totalIn)
	fmt.Fprintf(w, "- Total output tokens: %d\n\n", totalOut)

	stepIdx := map[int]*stepStat{}
	for _, row := range rep.Rows {
		s, ok := stepIdx[row.Step]
		if !ok {
			s = &stepStat{step: row.Step, concurrency: row.Concurrency, startMS: row.TSMS}
			stepIdx[row.Step] = s
		}
		s.count++
		if row.TSMS > s.endMS {
			s.endMS = row.TSMS
		}
		if row.TSMS < s.startMS || s.startMS == 0 {
			s.startMS = row.TSMS
		}
		switch {
		case row.Error != "" && row.HTTPCode == 0:
			s.terr++
		case row.HTTPCode >= 500:
			s.c5xx++
		case row.HTTPCode >= 400:
			s.c4xx++
		case row.HTTPCode >= 200 && row.HTTPCode < 400:
			s.ok++
		}
		s.latencies = append(s.latencies, row.LatencyMS)
		s.tokIn += row.InTokens
		s.tokOut += row.OutTokens
	}
	steps := make([]int, 0, len(stepIdx))
	for k := range stepIdx {
		steps = append(steps, k)
	}
	sort.Ints(steps)

	fmt.Fprintf(w, "## Per-step\n\n")
	fmt.Fprintf(w, "| Step | C | Duration | Reqs | OK | 4xx | 5xx | err | RPS | p50 ms | p95 ms | tok in/out avg |\n")
	fmt.Fprintf(w, "|------|---|----------|------|----|-----|-----|-----|-----|--------|--------|----------------|\n")
	for _, k := range steps {
		s := stepIdx[k]
		var rps float64
		if s.endMS > s.startMS {
			rps = float64(s.count) * 1000.0 / float64(s.endMS-s.startMS)
		}
		dms := s.endMS - s.startMS
		var tokInAvg, tokOutAvg int
		if s.count > 0 {
			tokInAvg = s.tokIn / s.count
			tokOutAvg = s.tokOut / s.count
		}
		fmt.Fprintf(w, "| %d | %d | %ds | %d | %d | %d | %d | %d | %.2f | %d | %d | %d / %d |\n",
			s.step, s.concurrency, dms/1000, s.count, s.ok, s.c4xx, s.c5xx, s.terr,
			rps, percentile(s.latencies, 0.5), percentile(s.latencies, 0.95),
			tokInAvg, tokOutAvg)
	}

	mIdx := map[string]*modelStat{}
	for _, row := range rep.Rows {
		m, ok := mIdx[row.Model]
		if !ok {
			m = &modelStat{model: row.Model}
			mIdx[row.Model] = m
		}
		m.count++
		if row.HTTPCode >= 200 && row.HTTPCode < 400 {
			m.ok++
		} else if row.HTTPCode >= 400 && row.HTTPCode < 500 {
			m.c4xx++
		}
		m.totLatency += int64(row.LatencyMS)
		m.tokIn += row.InTokens
		m.tokOut += row.OutTokens
	}
	models := make([]string, 0, len(mIdx))
	for k := range mIdx {
		models = append(models, k)
	}
	sort.Strings(models)

	fmt.Fprintf(w, "\n## Per-model\n\n")
	fmt.Fprintf(w, "| Model | Reqs | OK | 4xx | Avg latency ms | tok in/out avg |\n")
	fmt.Fprintf(w, "|-------|------|----|----|----------------|----------------|\n")
	for _, k := range models {
		m := mIdx[k]
		var avgLat int64
		var tokInAvg, tokOutAvg int
		if m.count > 0 {
			avgLat = m.totLatency / int64(m.count)
			tokInAvg = m.tokIn / m.count
			tokOutAvg = m.tokOut / m.count
		}
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %d / %d |\n",
			m.model, m.count, m.ok, m.c4xx, avgLat, tokInAvg, tokOutAvg)
	}

	ipIdx := map[string]*ipStat{}
	for _, row := range rep.Rows {
		if row.ExitIP == "" {
			continue
		}
		ip, ok := ipIdx[row.ExitIP]
		if !ok {
			ip = &ipStat{ip: row.ExitIP, firstStep: row.Step, lastStep: row.Step}
			ipIdx[row.ExitIP] = ip
		}
		ip.count++
		if row.HTTPCode >= 200 && row.HTTPCode < 400 {
			ip.ok++
		} else if row.HTTPCode >= 400 && row.HTTPCode < 500 {
			ip.c4xx++
		}
		if row.Step < ip.firstStep {
			ip.firstStep = row.Step
		}
		if row.Step > ip.lastStep {
			ip.lastStep = row.Step
		}
	}
	ips := make([]string, 0, len(ipIdx))
	for k := range ipIdx {
		ips = append(ips, k)
	}
	sort.Slice(ips, func(i, j int) bool { return ipIdx[ips[i]].count > ipIdx[ips[j]].count })

	fmt.Fprintf(w, "\n## Exit IP histogram\n\n")
	fmt.Fprintf(w, "| Exit IP | Reqs | OK | 4xx | First step | Last step |\n")
	fmt.Fprintf(w, "|---------|------|----|----|-----------|-----------|\n")
	for _, k := range ips {
		ip := ipIdx[k]
		fmt.Fprintf(w, "| `%s` | %d | %d | %d | %d | %d |\n", ip.ip, ip.count, ip.ok, ip.c4xx, ip.firstStep, ip.lastStep)
	}

	type errKey struct {
		code int
		msg  string
	}
	errIdx := map[errKey]int{}
	for _, row := range rep.Rows {
		if row.Error == "" && row.HTTPCode < 400 {
			continue
		}
		msg := row.Error
		if msg == "" {
			msg = firstLine(row.Response)
		}
		errIdx[errKey{code: row.HTTPCode, msg: truncate(msg, 80)}]++
	}
	type errRow struct {
		code  int
		msg   string
		count int
	}
	errRows := make([]errRow, 0, len(errIdx))
	for k, v := range errIdx {
		errRows = append(errRows, errRow{code: k.code, msg: k.msg, count: v})
	}
	sort.Slice(errRows, func(i, j int) bool { return errRows[i].count > errRows[j].count })

	fmt.Fprintf(w, "\n## Errors detail\n\n")
	if len(errRows) == 0 {
		fmt.Fprintf(w, "_No errors recorded._\n")
	} else {
		fmt.Fprintf(w, "| Code | Count | Sample message |\n")
		fmt.Fprintf(w, "|------|-------|----------------|\n")
		for _, er := range errRows {
			fmt.Fprintf(w, "| %d | %d | %s |\n", er.code, er.count, er.msg)
		}
	}

	fmt.Fprintf(w, "\n## Caveats\n")
	fmt.Fprintf(w, "- `exit_ip` precision is ~1 second (sidecar ipify-via-SOCKS5 sampler).\n")
	fmt.Fprintf(w, "  At high concurrency, multiple requests in the same second may share an IP tag while the real exit IP rotated within that second.\n")
	fmt.Fprintf(w, "- Test consumed real ChatGPT subscription resources. Account-level rate limits may persist for hours after this run.\n")
	return nil
}

func firstLine(r *RespBody) string {
	if r == nil {
		return ""
	}
	for i, c := range r.Content {
		if c == '\n' {
			return r.Content[:i]
		}
	}
	return r.Content
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
