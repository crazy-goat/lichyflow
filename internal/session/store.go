package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var validSessionID = regexp.MustCompile(`^[a-f0-9]+$`)

// Store manages session state on disk.
type Store struct {
	baseDir string
	mu      sync.RWMutex
}

// State represents the current state of a pipeline session.
type State struct {
	SessionID    string            `json:"session_id"`
	PipelineName string            `json:"pipeline_name"`
	CurrentState string            `json:"current_state"`
	StartedAt    time.Time         `json:"started_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	RetryCount   map[string]int    `json:"retry_count"`
	Flags        map[string]bool   `json:"flags"`
	Values       map[string]string `json:"values"`
	Artifacts   map[string]string `json:"artifacts"`
	ExitCode    int               `json:"exit_code,omitempty"`
	Exited      bool              `json:"exited"`
}

// NewStore creates a new session store rooted at baseDir.
func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

// NewSession creates a new session with a unique ID.
func (s *Store) NewSession(pipelineName, startState string) (*State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := generateID()
	sessionDir := s.sessionDir(id)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(sessionDir, "flags"), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(sessionDir, "values"), 0755); err != nil {
		return nil, err
	}

	state := &State{
		SessionID:    id,
		PipelineName: pipelineName,
		CurrentState: startState,
		StartedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
		RetryCount:   make(map[string]int),
		Flags:        make(map[string]bool),
		Values:       make(map[string]string),
		Artifacts:   make(map[string]string),
	}

	if err := s.save(state); err != nil {
		return nil, err
	}
	return state, nil
}

// Load reads an existing session from disk.
func (s *Store) Load(sessionID string) (*State, error) {
	if err := s.validateID(sessionID); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.statePath(sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse session state: %w", err)
	}
	return &state, nil
}

// Save persists session state to disk.
func (s *Store) Save(state *State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(state)
}

func (s *Store) save(state *State) error {
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return os.WriteFile(s.statePath(state.SessionID), data, 0644)
}

// SetFlag sets a boolean flag in the session.
func (s *Store) SetFlag(sessionID, name string, value bool) error {
	if err := s.validateID(sessionID); err != nil {
		return err
	}
	return s.readModifyWrite(sessionID, func(state *State) error {
		state.Flags[name] = value
		// Also write to a file for shell access
		flagDir := filepath.Join(s.sessionDir(sessionID), "flags")
		if value {
			if err := os.WriteFile(filepath.Join(flagDir, name), []byte("1"), 0644); err != nil {
				return err
			}
		} else {
			if err := os.Remove(filepath.Join(flagDir, name)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
}

// GetFlag reads a boolean flag from the session.
func (s *Store) GetFlag(sessionID, name string) (bool, error) {
	if err := s.validateID(sessionID); err != nil {
		return false, err
	}
	state, err := s.Load(sessionID)
	if err != nil {
		return false, err
	}
	v, ok := state.Flags[name]
	if !ok {
		return false, nil
	}
	return v, nil
}

// SetValue sets a string value in the session.
func (s *Store) SetValue(sessionID, name, value string) error {
	if err := s.validateID(sessionID); err != nil {
		return err
	}
	return s.readModifyWrite(sessionID, func(state *State) error {
		state.Values[name] = value
		// Also write to a file for shell access
		valDir := filepath.Join(s.sessionDir(sessionID), "values")
		if err := os.WriteFile(filepath.Join(valDir, name), []byte(value), 0644); err != nil {
			return err
		}
		return nil
	})
}

// GetValue reads a string value from the session.
func (s *Store) GetValue(sessionID, name string) (string, error) {
	if err := s.validateID(sessionID); err != nil {
		return "", err
	}
	state, err := s.Load(sessionID)
	if err != nil {
		return "", err
	}
	v, ok := state.Values[name]
	if !ok {
		return "", nil
	}
	return v, nil
}

// SetArtifact registers an artifact path in the session.
func (s *Store) SetArtifact(sessionID, name, path string) error {
	if err := s.validateID(sessionID); err != nil {
		return err
	}
	return s.readModifyWrite(sessionID, func(state *State) error {
		state.Artifacts[name] = path
		return nil
	})
}

// IncrementRetry increments the retry counter for a step.
func (s *Store) IncrementRetry(sessionID, stepName string) (int, error) {
	if err := s.validateID(sessionID); err != nil {
		return 0, err
	}
	var count int
	if err := s.readModifyWrite(sessionID, func(state *State) error {
		state.RetryCount[stepName]++
		count = state.RetryCount[stepName]
		return nil
	}); err != nil {
		return 0, err
	}
	return count, nil
}

// GetRetryCount returns the current retry count for a step.
func (s *Store) GetRetryCount(sessionID, stepName string) (int, error) {
	if err := s.validateID(sessionID); err != nil {
		return 0, err
	}
	state, err := s.Load(sessionID)
	if err != nil {
		return 0, err
	}
	return state.RetryCount[stepName], nil
}

// EnvVars returns the environment variables for a step in this session.
// Each variable is exported both with and without the LICHYFLOW_ prefix
// for convenience (e.g. LICHYFLOW_ARTIFACT_DIR and ARTIFACT_DIR).
func (s *Store) EnvVars(sessionID, pipelineName, currentState string) map[string]string {
	if err := s.validateID(sessionID); err != nil {
		// Return empty map so callers don't get poisoned paths
		return map[string]string{}
	}
	sessionDir := s.sessionDir(sessionID)
	vars := map[string]string{
		"LICHYFLOW_SESSION_ID":    sessionID,
		"LICHYFLOW_PIPELINE":     pipelineName,
		"LICHYFLOW_STATE":         currentState,
		"LICHYFLOW_STORE":          s.baseDir,
		"LICHYFLOW_ARTIFACT_DIR": sessionDir,
		"LICHYFLOW_FLAGS_DIR":    filepath.Join(sessionDir, "flags"),
		"LICHYFLOW_VALUES_DIR":   filepath.Join(sessionDir, "values"),
	}
	// Add short aliases (without LICHYFLOW_ prefix)
	for k, v := range vars {
		alias := strings.TrimPrefix(k, "LICHYFLOW_")
		vars[alias] = v
	}
	return vars
}

func (s *Store) validateID(id string) error {
	if !validSessionID.MatchString(id) {
		return fmt.Errorf("invalid session ID: %q must be hex-only", id)
	}
	return nil
}

// readModifyWrite acquires the write lock, loads state, calls the modifier function,
// saves the state, and releases the lock. This prevents TOCTOU races.
func (s *Store) readModifyWrite(sessionID string, modifier func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.loadInternal(sessionID)
	if err != nil {
		return err
	}
	if err := modifier(state); err != nil {
		return err
	}
	return s.save(state)
}

// loadInternal loads state without acquiring a lock (caller must hold mu).
func (s *Store) loadInternal(sessionID string) (*State, error) {
	path := s.statePath(sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session state: %w", err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse session state: %w", err)
	}
	return &state, nil
}

func (s *Store) sessionDir(id string) string {
	return filepath.Join(s.baseDir, "sessions", id)
}

func (s *Store) statePath(id string) string {
	return filepath.Join(s.sessionDir(id), "state.json")
}

// BaseDir returns the base directory for sessions.
func (s *Store) BaseDir() string {
	return s.baseDir
}

func generateID() string {
	// Simple hex ID based on timestamp
	return fmt.Sprintf("%x", time.Now().UnixNano())
}