package main

import "time"

// StopReason describes why the test ended.
type StopReason string

const (
	StopComplete  StopReason = "complete"
	StopErrorRate StopReason = "error_rate_exceeded"
	StopNoSuccess StopReason = "no_success_30s"
	StopTimeLimit StopReason = "time_limit"
	StopSignal    StopReason = "signal"
)

// Eval holds parameters for evaluating stop conditions.
type Eval struct {
	StartTime          time.Time
	HardLimit          time.Duration
	ErrorRateThreshold float64
	NoSuccessWindow    time.Duration
}

func isFailure(r Row) bool {
	if r.Error != "" {
		return true
	}
	if r.HTTPCode == 0 {
		return true
	}
	if r.HTTPCode >= 400 {
		return true
	}
	return false
}

// EvalStep returns a stop reason if the step's rows breach the error-rate threshold.
func (e *Eval) EvalStep(rows []Row) StopReason {
	if len(rows) == 0 {
		return ""
	}
	fails := 0
	for _, r := range rows {
		if isFailure(r) {
			fails++
		}
	}
	rate := float64(fails) / float64(len(rows))
	if rate >= e.ErrorRateThreshold {
		return StopErrorRate
	}
	return ""
}

// EvalGlobal checks time-limit and no-success-in-window conditions.
func (e *Eval) EvalGlobal(now time.Time, recent []Row) StopReason {
	if e.HardLimit > 0 && now.Sub(e.StartTime) >= e.HardLimit {
		return StopTimeLimit
	}
	if e.NoSuccessWindow == 0 {
		return ""
	}
	cutoff := now.Add(-e.NoSuccessWindow).UnixMilli()
	hadSuccess := false
	hadAny := false
	for _, r := range recent {
		if r.TSMS < cutoff {
			continue
		}
		hadAny = true
		if !isFailure(r) {
			hadSuccess = true
			break
		}
	}
	if hadAny && !hadSuccess {
		return StopNoSuccess
	}
	return ""
}
