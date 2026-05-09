package prober

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/fsnotify/fsnotify"
)

// PassiveTail follows a log file, parses HTTP status codes, and emits them.
type PassiveTail struct {
	Path    string
	Pattern string // regex with one capture group for the status code
	Emit    func(ts time.Time, code int)
	Log     *slog.Logger

	rx *regexp.Regexp
}

func (p *PassiveTail) compile() error {
	rx, err := regexp.Compile(p.Pattern)
	if err != nil {
		return err
	}
	p.rx = rx
	return nil
}

func (p *PassiveTail) Run(ctx context.Context) error {
	if err := p.compile(); err != nil {
		return err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := w.Add(p.Path); err != nil {
		return err
	}

	f, err := os.Open(p.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	// seek to end
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	br := bufio.NewReader(f)

	readNew := func() {
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					return
				}
				if p.Log != nil {
					p.Log.Error("passive read", "err", err)
				}
				return
			}
			m := p.rx.FindStringSubmatch(line)
			if len(m) >= 2 {
				code, _ := strconv.Atoi(m[1])
				if p.Emit != nil {
					p.Emit(time.Now(), code)
				}
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op&fsnotify.Write == fsnotify.Write {
				readNew()
			}
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			if p.Log != nil {
				p.Log.Error("watcher", "err", err)
			}
		}
	}
}
