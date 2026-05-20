package session_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lichyflow/lichyflow/internal/session"
)

func TestNewSession(t *testing.T) {
	tmpDir := t.TempDir()
	store := session.NewStore(tmpDir)

	state, err := store.NewSession("test-pipeline", "start")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if state.SessionID == "" {
		t.Error("SessionID is empty")
	}
	if state.PipelineName != "test-pipeline" {
		t.Errorf("expected PipelineName 'test-pipeline', got %q", state.PipelineName)
	}
	if state.CurrentState != "start" {
		t.Errorf("expected CurrentState 'start', got %q", state.CurrentState)
	}
	if state.Exited {
		t.Error("new session should not be exited")
	}
}

func TestSetAndGetFlag(t *testing.T) {
	tmpDir := t.TempDir()
	store := session.NewStore(tmpDir)

	state, _ := store.NewSession("test", "start")
	sid := state.SessionID

	// Initially false
	v, err := store.GetFlag(sid, "my_flag")
	if err != nil {
		t.Fatalf("GetFlag: %v", err)
	}
	if v {
		t.Error("expected flag to be false initially")
	}

	// Set to true
	if err := store.SetFlag(sid, "my_flag", true); err != nil {
		t.Fatalf("SetFlag: %v", err)
	}

	v, err = store.GetFlag(sid, "my_flag")
	if err != nil {
		t.Fatalf("GetFlag: %v", err)
	}
	if !v {
		t.Error("expected flag to be true after setting")
	}

	// Set to false
	if err := store.SetFlag(sid, "my_flag", false); err != nil {
		t.Fatalf("SetFlag false: %v", err)
	}

	v, err = store.GetFlag(sid, "my_flag")
	if err != nil {
		t.Fatalf("GetFlag: %v", err)
	}
	if v {
		t.Error("expected flag to be false after unsetting")
	}

	// Check flag file exists
	flagPath := filepath.Join(tmpDir, "sessions", sid, "flags", "my_flag")
	if _, err := os.Stat(filepath.Join(tmpDir, "sessions", sid, "flags")); err != nil {
		t.Errorf("flags dir should exist: %v", err)
	}
	_ = flagPath // file only created when flag is true
}

func TestSetAndGetValue(t *testing.T) {
	tmpDir := t.TempDir()
	store := session.NewStore(tmpDir)

	state, _ := store.NewSession("test", "start")
	sid := state.SessionID

	// Initially empty
	v, err := store.GetValue(sid, "counter")
	if err != nil {
		t.Fatalf("GetValue: %v", err)
	}
	if v != "" {
		t.Errorf("expected empty value initially, got %q", v)
	}

	// Set value
	if err := store.SetValue(sid, "counter", "42"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	v, err = store.GetValue(sid, "counter")
	if err != nil {
		t.Fatalf("GetValue: %v", err)
	}
	if v != "42" {
		t.Errorf("expected '42', got %q", v)
	}

	// Override value
	if err := store.SetValue(sid, "counter", "100"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	v, _ = store.GetValue(sid, "counter")
	if v != "100" {
		t.Errorf("expected '100', got %q", v)
	}
}

func TestEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	store := session.NewStore(tmpDir)

	state, _ := store.NewSession("my-pipeline", "lint")
	sid := state.SessionID

	env := store.EnvVars(sid, "my-pipeline", "lint")

	if env["LICHYFLOW_SESSION_ID"] != sid {
		t.Errorf("expected LICHYFLOW_SESSION_ID=%s, got %s", sid, env["LICHYFLOW_SESSION_ID"])
	}
	if env["LICHYFLOW_PIPELINE"] != "my-pipeline" {
		t.Errorf("expected LICHYFLOW_PIPELINE=my-pipeline, got %s", env["LICHYFLOW_PIPELINE"])
	}
	if env["LICHYFLOW_STATE"] != "lint" {
		t.Errorf("expected LICHYFLOW_STATE=lint, got %s", env["LICHYFLOW_STATE"])
	}
}

func TestRetryCount(t *testing.T) {
	tmpDir := t.TempDir()
	store := session.NewStore(tmpDir)

	state, _ := store.NewSession("test", "start")
	sid := state.SessionID

	count, _ := store.GetRetryCount(sid, "step1")
	if count != 0 {
		t.Errorf("expected 0 retries, got %d", count)
	}

	count, _ = store.IncrementRetry(sid, "step1")
	if count != 1 {
		t.Errorf("expected 1 retry, got %d", count)
	}

	count, _ = store.IncrementRetry(sid, "step1")
	if count != 2 {
		t.Errorf("expected 2 retries, got %d", count)
	}

	count, _ = store.GetRetryCount(sid, "step1")
	if count != 2 {
		t.Errorf("expected 2 retries, got %d", count)
	}
}