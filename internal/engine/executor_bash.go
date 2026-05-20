package engine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/lichyflow/lichyflow/internal/pipeline"
	"github.com/lichyflow/lichyflow/internal/session"
)

// BashExecutor runs bash commands as pipeline steps.
type BashExecutor struct {
	store *session.Store
}

// NewBashExecutor creates a new bash executor.
func NewBashExecutor(store *session.Store) *BashExecutor {
	return &BashExecutor{store: store}
}

// Run executes a bash step and returns the exit code.
func (b *BashExecutor) Run(ctx context.Context, state *session.State, step *pipeline.Step) (int, error) {
	// Build environment variables
	env := b.buildEnv(state, step)

	// Determine timeout
	timeout := 300 // default 5 minutes
	if step.Timeout > 0 {
		timeout = step.Timeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Execute the command with process group isolation
		cmd := exec.CommandContext(ctx, "bash", "-e", "-c", step.Command)
	cmd.Env = append(os.Environ(), envToSlice(env)...)
	// Run in current working directory (where lichyflow was invoked)

	// Create a new process group so we can kill all subprocesses
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Kill the whole process group (not just the leader)
		if cmd.Process != nil && cmd.Process.Pid > 0 {
			pgid, err := syscall.Getpgid(cmd.Process.Pid)
			if err == nil && pgid > 0 {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			}
		}
		return nil
	}

	var stdout, stderr bytes.Buffer
	// Prefix output lines with step name for clarity
	stdoutPrefix := fmt.Sprintf("[%s] ", step.Name)
	cmd.Stdout = io.MultiWriter(&stdout, newPrefixWriter(os.Stdout, stdoutPrefix))
	stderrPrefix := fmt.Sprintf("[%s] ", step.Name)
	cmd.Stderr = io.MultiWriter(&stderr, newPrefixWriter(os.Stderr, stderrPrefix))

	log.Printf("[%s] Executing bash step %q", state.SessionID, step.Name)

	err := cmd.Run()

	// Check for timeout
	if ctx.Err() == context.DeadlineExceeded {
		log.Printf("[%s] Step %q timed out after %ds", state.SessionID, step.Name, timeout)
		return 2, nil // exit code 2 = timeout (custom exit)
	}

	if err != nil {
		// Extract exit code
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			if code == -1 {
				code = 1
			}
			log.Printf("[%s] Step %q exited with code %d", state.SessionID, step.Name, code)
			log.Printf("[%s] stderr: %s", state.SessionID, stderr.String())
			return code, nil
		}
		return 1, fmt.Errorf("execute step %q: %w", step.Name, err)
	}

	log.Printf("[%s] Step %q exited with code 0", state.SessionID, step.Name)
	return 0, nil
}

// buildEnv constructs environment variables for a step.
func (b *BashExecutor) buildEnv(state *session.State, step *pipeline.Step) map[string]string {
	env := b.store.EnvVars(state.SessionID, state.PipelineName, state.CurrentState)

	// Add declared retry count
	retryCount, _ := b.store.GetRetryCount(state.SessionID, step.Name)
	env["LICHYFLOW_RETRY_COUNT"] = fmt.Sprintf("%d", retryCount)
	env["RETRY_COUNT"] = fmt.Sprintf("%d", retryCount)

	// Add step-specific env vars (override session vars)
	for k, v := range step.Env {
		env[k] = v
	}

	return env
}

func envToSlice(env map[string]string) []string {
	slice := make([]string, 0, len(env))
	for k, v := range env {
		slice = append(slice, fmt.Sprintf("%s=%s", k, v))
	}
	return slice
}

// prefixWriter wraps an io.Writer and prepends a prefix to each line.
type prefixWriter struct {
	w      io.Writer
	prefix string
	bol    bool // at beginning of line
}

func newPrefixWriter(w io.Writer, prefix string) *prefixWriter {
	return &prefixWriter{w: w, prefix: prefix, bol: true}
}

func (p *prefixWriter) Write(buf []byte) (int, error) {
	var written int
	for len(buf) > 0 {
		if p.bol {
			if _, err := p.w.Write([]byte(p.prefix)); err != nil {
				return written, err
			}
			p.bol = false
		}
		// Find next newline
		i := bytes.IndexByte(buf, '\n')
		if i == -1 {
			n, err := p.w.Write(buf)
			written += n
			if err != nil {
				return written, err
			}
			return written, nil
		}
		// Write up to and including newline
		n, err := p.w.Write(buf[:i+1])
		written += n
		if err != nil {
			return written, err
		}
		buf = buf[i+1:]
		p.bol = true
	}
	return written, nil
}