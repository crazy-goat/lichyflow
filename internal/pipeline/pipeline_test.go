package pipeline_test

import (
	"testing"

	"github.com/lichyflow/lichyflow/internal/pipeline"
)

func TestLoadSimpleCI(t *testing.T) {
	p, err := pipeline.LoadFromFile("../../examples/01-simple-ci.yaml")
	if err != nil {
		t.Fatalf("load pipeline: %v", err)
	}

	if p.Name != "simple-ci" {
		t.Errorf("expected name 'simple-ci', got %q", p.Name)
	}
	if p.Start != "lint" {
		t.Errorf("expected start 'lint', got %q", p.Start)
	}
	if len(p.Steps) != 3 {
		t.Errorf("expected 3 steps, got %d", len(p.Steps))
	}
	if len(p.Transitions) != 3 {
		t.Errorf("expected 3 transitions, got %d", len(p.Transitions))
	}

	// Check default exits
	exits := p.AllExits()
	if len(exits) < 2 {
		t.Errorf("expected at least 2 default exits, got %d", len(exits))
	}
	found := map[string]bool{}
	for _, e := range exits {
		found[e.Name] = true
	}
	if !found["success"] || !found["failed"] {
		t.Error("default exits 'success' and 'failed' must be present")
	}
}

func TestLoadRetryCounter(t *testing.T) {
	p, err := pipeline.LoadFromFile("../../examples/02-retry-counter.yaml")
	if err != nil {
		t.Fatalf("load pipeline: %v", err)
	}

	if p.Name != "retry-counter" {
		t.Errorf("expected name 'retry-counter', got %q", p.Name)
	}

	// Check init step
	initStep := p.GetStep("init")
	if initStep == nil {
		t.Fatal("init step not found")
	}
	if initStep.Type != "bash" {
		t.Errorf("expected init type 'bash', got %q", initStep.Type)
	}

	// Check check step has retries
	checkStep := p.GetStep("check")
	if checkStep == nil {
		t.Fatal("check step not found")
	}
	if checkStep.Retry.Max != 3 {
		t.Errorf("expected max retries 3, got %d", checkStep.Retry.Max)
	}
	if checkStep.Retry.Backoff != "fixed" {
		t.Errorf("expected backoff 'fixed', got %q", checkStep.Retry.Backoff)
	}
}

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "missing name",
			yaml: `pipeline:
  start: foo
  steps:
    - name: foo
      type: bash
      command: echo hi`,
			wantErr: "name is required",
		},
		{
			name: "missing start",
			yaml: `pipeline:
  name: test
  steps:
    - name: foo
      type: bash
      command: echo hi`,
			wantErr: "start is required",
		},
		{
			name: "start not in steps",
			yaml: `pipeline:
  name: test
  start: missing
  steps:
    - name: foo
      type: bash
      command: echo hi`,
			wantErr: "not found",
		},
		{
			name: "custom exit uses reserved code 0",
			yaml: `pipeline:
  name: test
  start: foo
  exits:
    - name: custom_success
      exit_code: 0
  steps:
    - name: foo
      type: bash
      command: echo hi`,
			wantErr: "reserved exit code 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pipeline.LoadFromBytes([]byte(tt.yaml))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			// Check that the error contains the expected substring
			if !contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}