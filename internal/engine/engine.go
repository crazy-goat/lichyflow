package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"time"

	"github.com/looplab/fsm"

	"github.com/lichyflow/lichyflow/internal/pipeline"
	"github.com/lichyflow/lichyflow/internal/session"
)

// Engine orchestrates pipeline execution using a state machine.
type Engine struct {
	pipeline *pipeline.Pipeline
	store    *session.Store
	fsm      *fsm.FSM
}

// New creates a new Engine for the given pipeline.
func New(p *pipeline.Pipeline, store *session.Store) *Engine {
	return &Engine{
		pipeline: p,
		store:    store,
	}
}

// Run executes the pipeline from start to an exit state.
func (e *Engine) Run(ctx context.Context) (int, error) {
	// Create a new session
	state, err := e.store.NewSession(e.pipeline.Name, e.pipeline.Start)
	if err != nil {
		return 1, fmt.Errorf("create session: %w", err)
	}

	log.Printf("[%s] Starting pipeline %q from state %q", state.SessionID, e.pipeline.Name, e.pipeline.Start)

	// Build the FSM
	e.fsm = e.buildFSM(state)

	// Check requirements
	if err := e.checkRequirements(); err != nil {
		return 1, fmt.Errorf("requirements: %w", err)
	}

	// Execute steps until we reach an exit
	current := e.pipeline.Start
	for {
		// Check for context cancellation (SIGINT/SIGTERM)
		select {
		case <-ctx.Done():
			log.Printf("[%s] Pipeline cancelled", state.SessionID)
			return 1, ctx.Err()
		default:
		}

		// Check if we've reached an exit
		if e.pipeline.IsExit(current) {
			exitCode := e.pipeline.GetExitCode(current)
			state.Exited = true
			state.ExitCode = exitCode
			if err := e.store.Save(state); err != nil {
				return exitCode, fmt.Errorf("save state: %w", err)
			}
			log.Printf("[%s] Pipeline exited at %q with code %d", state.SessionID, current, exitCode)
			return exitCode, nil
		}

		// Get the step to execute
		step := e.pipeline.GetStep(current)
		if step == nil {
			return 1, fmt.Errorf("step %q not found", current)
		}

		// Determine retry config with defaults
		maxRetry := step.Retry.Max
		if maxRetry < 0 {
			maxRetry = 0
		}

		// Execute the step with retries
		var next string
		next, err = e.executeStepWithRetries(ctx, state, step, maxRetry)
		if err != nil {
			return 1, err
		}

		// Transition to next state
		current = next
		state.CurrentState = current
		if err := e.store.Save(state); err != nil {
			return e.pipeline.GetExitCode("failed"), fmt.Errorf("save state: %w", err)
		}
	}
}

// executeStepWithRetries runs a step, retrying on failure up to maxRetry times.
// After all retries (or if no retries configured), it checks conditional transitions
// to find the next state based on flags/values set by the step.
func (e *Engine) executeStepWithRetries(ctx context.Context, state *session.State, step *pipeline.Step, maxRetry int) (string, error) {
	for attempt := 0; attempt <= maxRetry; attempt++ {
		if attempt > 0 {
			delay := e.retryDelay(step, attempt)
			log.Printf("[%s] Retrying step %q (attempt %d/%d, delay %v)", state.SessionID, step.Name, attempt, maxRetry, delay)
			time.Sleep(delay)
		}

		exitCode, err := e.executeStep(ctx, state, step)
		if err != nil {
			return "", fmt.Errorf("execute step %q: %w", step.Name, err)
		}

		if exitCode == 0 {
			log.Printf("[%s] Step %q passed (attempt %d)", state.SessionID, step.Name, attempt+1)
			return e.findNextState(state, step, true)
		}

		// Step failed
		log.Printf("[%s] Step %q failed with exit code %d (attempt %d/%d)", state.SessionID, step.Name, exitCode, attempt+1, maxRetry+1)

		if attempt < maxRetry {
			// Record retry count
			if _, err := e.store.IncrementRetry(state.SessionID, step.Name); err != nil {
				log.Printf("[%s] failed to increment retry count for step %q: %v", state.SessionID, step.Name, err)
			}
		}
	}

	// All retries exhausted (or no retries configured)
	// Check conditional transitions — the step may have set flags/values
	// that route to a different state (e.g. run_tests → llm_fix_tests)
	log.Printf("[%s] Step %q finished with failure, checking conditionals", state.SessionID, step.Name)
	next, err := e.findNextState(state, step, false)
	if err != nil {
		return "", err
	}
	if next != "failed" {
		// Found a conditional transition for failure
		log.Printf("[%s] Step %q → %q (conditional failure transition)", state.SessionID, step.Name, next)
		return next, nil
	}

	// No conditional transition found, go to failed
	log.Printf("[%s] Step %q exhausted all retries (%d) and no conditional failure transition", state.SessionID, step.Name, maxRetry+1)
	return "failed", nil
}

// retryDelay calculates the delay before the next retry attempt.
func (e *Engine) retryDelay(step *pipeline.Step, attempt int) time.Duration {
	initialDelay := 5 * time.Second
	if step.Retry.InitialDelay != "" {
		if d, err := time.ParseDuration(step.Retry.InitialDelay); err == nil {
			initialDelay = d
		}
	}

	switch step.Retry.Backoff {
	case "fixed":
		return initialDelay
	case "exponential", "":
		// 5s, 10s, 20s, 40s...
		return initialDelay * time.Duration(1<<uint(attempt-1))
	default:
		return initialDelay
	}
}

// findNextState determines the next state based on step result and conditions.
// Priority: conditional transitions > unconditional transitions > defaults
func (e *Engine) findNextState(state *session.State, step *pipeline.Step, success bool) (string, error) {
	// First: check conditional transitions (they take priority on success AND failure)
	for _, t := range e.pipeline.Transitions {
		if t.Src != step.Name {
			continue
		}
		if t.Condition != nil && e.evaluateCondition(state, t.Condition) {
			return t.Dst, nil
		}
	}

	// Second: check unconditional transitions (only on success)
	if success {
		for _, t := range e.pipeline.Transitions {
			if t.Src != step.Name {
				continue
			}
			if t.Condition == nil {
				return t.Dst, nil
			}
		}
	}

	// Default: on success go to "success", on failure go to "failed"
	if success {
		return "success", nil
	}
	return "failed", nil
}

// evaluateCondition checks if a transition condition is met.
func (e *Engine) evaluateCondition(state *session.State, cond *pipeline.Condition) bool {
	result := false

	if cond.Flag != "" {
		v, err := e.store.GetFlag(state.SessionID, cond.Flag)
		if err != nil {
			log.Printf("[%s] evaluateCondition: failed to get flag %q: %v", state.SessionID, cond.Flag, err)
		}
		result = v
	}

	if cond.Value != "" {
		v, err := e.store.GetValue(state.SessionID, cond.Value)
		if err != nil {
			log.Printf("[%s] evaluateCondition: failed to get value %q: %v", state.SessionID, cond.Value, err)
			result = false
		} else {
			// Count how many comparison fields are set to warn about overlapping conditions
			setCount := 0
			if cond.Greater != "" {
				setCount++
			}
			if cond.Less != "" {
				setCount++
			}
			if cond.Equal != "" {
				setCount++
			}
			if setCount > 1 {
				log.Printf("[%s] evaluateCondition: warning — multiple comparison fields set on condition (greater=%q, less=%q, equal=%q), only first will be evaluated", state.SessionID, cond.Greater, cond.Less, cond.Equal)
			}

			switch {
			case cond.Greater != "":
				vInt, errV := strconv.Atoi(v)
				condInt, errC := strconv.Atoi(cond.Greater)
				if errV == nil && errC == nil {
					result = vInt > condInt
				} else {
					log.Printf("[%s] evaluateCondition: cannot parse numeric comparison for greater: value=%q, greater=%q (errV=%v, errC=%v)", state.SessionID, v, cond.Greater, errV, errC)
					result = false
				}
			case cond.Less != "":
				vInt, errV := strconv.Atoi(v)
				condInt, errC := strconv.Atoi(cond.Less)
				if errV == nil && errC == nil {
					result = vInt < condInt
				} else {
					log.Printf("[%s] evaluateCondition: cannot parse numeric comparison for less: value=%q, less=%q (errV=%v, errC=%v)", state.SessionID, v, cond.Less, errV, errC)
					result = false
				}
			case cond.Equal != "":
				// Try integer comparison first for consistency with Greater/Less
				if vInt, errV := strconv.Atoi(v); errV == nil {
					if condInt, errC := strconv.Atoi(cond.Equal); errC == nil {
						result = vInt == condInt
						break
					}
				}
				// Fall back to string comparison if either side is non-numeric
				result = v == cond.Equal
			}
		}
	}

	if cond.Negate {
		return !result
	}
	return result
}

// executeStep runs a single step and returns its exit code.
func (e *Engine) executeStep(ctx context.Context, state *session.State, step *pipeline.Step) (int, error) {
	switch step.Type {
	case "bash":
		return e.executeBash(ctx, state, step)
	case "llm":
		return e.executeLLM(ctx, state, step)
	default:
		return 1, fmt.Errorf("unknown step type: %q", step.Type)
	}
}

// executeBash runs a bash step.
func (e *Engine) executeBash(ctx context.Context, state *session.State, step *pipeline.Step) (int, error) {
	executor := NewBashExecutor(e.store)
	return executor.Run(ctx, state, step)
}

// checkRequirements verifies that all required commands are available on the system.
func (e *Engine) checkRequirements() error {
	for _, req := range e.pipeline.Requirements {
		label := req.Name
		if label == "" {
			label = req.Command
		}
		if label == "" {
			label = req.Check
			if len(label) > 50 {
				label = label[:50] + "..."
			}
		}
		log.Printf("[requirements] checking: %s", label)

		var err error
		if req.Check != "" {
			// Run arbitrary bash check
			cmd := exec.Command("bash", "-c", req.Check)
			err = cmd.Run()
		} else if req.Command != "" {
			cmd := exec.Command("which", req.Command)
			err = cmd.Run()
		} else {
			return fmt.Errorf("requirement with no command or check")
		}

		if err != nil {
			msg := fmt.Sprintf("requirement failed: %s", label)
			if req.Hint != "" {
				msg += "\n  hint: " + req.Hint
			}
			return errors.New(msg)
		}
		log.Printf("[requirements] \u2713 %s", label)
	}
	return nil
}

// executeLLM runs an LLM step.
func (e *Engine) executeLLM(ctx context.Context, state *session.State, step *pipeline.Step) (int, error) {
	// TODO: implement LLM executor
	log.Printf("[%s] LLM step %q not yet implemented, skipping", state.SessionID, step.Name)
	return 0, nil
}

// buildFSM creates a looplab FSM from the pipeline definition.
func (e *Engine) buildFSM(state *session.State) *fsm.FSM {
	events := fsm.Events{}

	// Add transition events
	for _, t := range e.pipeline.Transitions {
		events = append(events, fsm.EventDesc{
			Name: t.Event,
			Src:  []string{t.Src},
			Dst:  t.Dst,
		})
	}

	// Add default transitions for steps without explicit transitions
	for _, s := range e.pipeline.Steps {
		hasTransition := false
		for _, t := range e.pipeline.Transitions {
			if t.Src == s.Name {
				hasTransition = true
				break
			}
		}
		if !hasTransition {
			// Auto-create: step -> next step or success
			events = append(events, fsm.EventDesc{
				Name: "next",
				Src:  []string{s.Name},
				Dst:  "success",
			})
		}
	}

	return fsm.NewFSM(
		state.CurrentState,
		events,
		fsm.Callbacks{},
	)
}

// Visualize returns a Mermaid diagram of the pipeline.
func (e *Engine) Visualize() (string, error) {
	if e.fsm == nil {
		state, err := e.store.NewSession("_viz", e.pipeline.Start)
		if err != nil {
			return "", fmt.Errorf("create viz session: %w", err)
		}
		e.fsm = e.buildFSM(state)
	}
	result, err := fsm.VisualizeForMermaidWithGraphType(e.fsm, fsm.StateDiagram)
	if err != nil {
		return "", fmt.Errorf("visualize: %w", err)
	}
	return result, nil
}