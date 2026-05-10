package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// StepConfig is one ramp step.
type StepConfig struct {
	Step        int
	Concurrency int
	Duration    time.Duration
}

// Runner drives the stair-step ramp test. All upstream interactions are pluggable
// via function fields so tests can mock them.
type Runner struct {
	Steps     []StepConfig
	Models    []string
	Tasks     []string
	Writer    *Writer
	MaxToks   int
	Temp      float64
	Eval      *Eval
	DoChat    func(ctx context.Context, req ChatRequest) ChatResult
	GetSample func() Sample

	seq atomic.Int64 // global request counter for round-robin model selection
}

// Run executes the configured steps until completion or stop.
func (r *Runner) Run(ctx context.Context, start time.Time) StopReason {
	r.Eval.StartTime = start

	for _, step := range r.Steps {
		if reason := r.Eval.EvalGlobal(time.Now(), nil); reason != "" {
			return reason
		}

		stepRows := r.runStep(ctx, step)

		if reason := r.Eval.EvalStep(stepRows); reason != "" {
			return reason
		}
		if reason := r.Eval.EvalGlobal(time.Now(), stepRows); reason != "" {
			return reason
		}
		if ctx.Err() != nil {
			return StopSignal
		}
	}
	return StopComplete
}

// runStep launches `step.Concurrency` workers for `step.Duration`, returns all rows produced.
func (r *Runner) runStep(ctx context.Context, step StepConfig) []Row {
	stepCtx, cancel := context.WithTimeout(ctx, step.Duration)
	defer cancel()

	var (
		mu   sync.Mutex
		rows []Row
		wg   sync.WaitGroup
	)
	collect := func(row Row) {
		mu.Lock()
		rows = append(rows, row)
		mu.Unlock()
	}

	for w := 0; w < step.Concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for stepCtx.Err() == nil {
				row := r.fireOne(stepCtx, step, workerID)
				_ = r.Writer.Write(row)
				collect(row)

				mu.Lock()
				snapshot := make([]Row, len(rows))
				copy(snapshot, rows)
				mu.Unlock()

				if reason := r.Eval.EvalGlobal(time.Now(), snapshot); reason != "" {
					cancel()
					return
				}
			}
		}(w)
	}
	wg.Wait()
	return rows
}

func (r *Runner) fireOne(ctx context.Context, step StepConfig, workerID int) Row {
	seq := r.seq.Add(1) - 1
	model := r.Models[seq%int64(len(r.Models))]

	task := r.Tasks[int(seq)%len(r.Tasks)]
	prompt := BuildPrompt(task)

	sample := r.GetSample()
	now := time.Now()

	res := r.DoChat(ctx, ChatRequest{
		Model:       model,
		Messages:    []Message{{Role: "user", Content: prompt}},
		MaxTokens:   r.MaxToks,
		Temperature: r.Temp,
	})

	row := Row{
		TSMS:        now.UnixMilli(),
		Step:        step.Step,
		Concurrency: step.Concurrency,
		WorkerID:    workerID,
		Model:       model,
		Prompt:      prompt,
		HTTPCode:    res.HTTPCode,
		LatencyMS:   res.LatencyMS,
		InTokens:    res.InTokens,
		OutTokens:   res.OutTokens,
		TotalTokens: res.TotalTokens,
		ExitIP:      sample.IP,
		Error:       res.Error,
	}
	if sample.TSMS != 0 {
		row.ExitIPAgeMS = int(now.UnixMilli() - sample.TSMS)
	}
	if res.HTTPCode == 200 || res.Content != "" {
		row.Response = &RespBody{
			ID:           res.ID,
			Content:      res.Content,
			FinishReason: res.FinishReason,
		}
	}
	return row
}
