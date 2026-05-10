package main

import (
	"strings"
	"testing"
)

func TestModelsList(t *testing.T) {
	want := []string{"gpt-5.2", "gpt-5.4", "gpt-5.4-mini", "gpt-5.5", "gpt-5.3-codex"}
	if len(Models) != len(want) {
		t.Fatalf("len(Models)=%d, want %d", len(Models), len(want))
	}
	for i, m := range want {
		if Models[i] != m {
			t.Errorf("Models[%d]=%q, want %q", i, Models[i], m)
		}
	}
}

func TestModelForRequestRoundRobin(t *testing.T) {
	for i, want := range []string{
		"gpt-5.2", "gpt-5.4", "gpt-5.4-mini", "gpt-5.5", "gpt-5.3-codex",
		"gpt-5.2", "gpt-5.4",
	} {
		got := ModelForRequest(int64(i))
		if got != want {
			t.Errorf("ModelForRequest(%d)=%q, want %q", i, got, want)
		}
	}
}

func TestTaskPoolNonEmpty(t *testing.T) {
	if len(Tasks) < 20 {
		t.Errorf("len(Tasks)=%d, want >=20", len(Tasks))
	}
	for i, task := range Tasks {
		if strings.TrimSpace(task) == "" {
			t.Errorf("Tasks[%d] is empty", i)
		}
	}
}

func TestBuildPrompt(t *testing.T) {
	p := BuildPrompt("reverses a string")
	if !strings.Contains(p, "reverses a string") {
		t.Errorf("expected task substring, got: %q", p)
	}
	if !strings.Contains(p, "Python function") {
		t.Errorf("expected 'Python function' in prompt, got: %q", p)
	}
}
