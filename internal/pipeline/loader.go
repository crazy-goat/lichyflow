package pipeline

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// pipelineFile is the top-level YAML structure.
// Everything lives under the "pipeline" key.
type pipelineFile struct {
	Pipeline Pipeline `yaml:"pipeline" json:"pipeline"`
}

// LoadFromFile reads a YAML pipeline definition from a file.
func LoadFromFile(path string) (*Pipeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pipeline file: %w", err)
	}
	return LoadFromBytes(data)
}

// LoadFromBytes parses a YAML pipeline definition.
func LoadFromBytes(data []byte) (*Pipeline, error) {
	var f pipelineFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse pipeline yaml: %w", err)
	}

	if err := f.Pipeline.Validate(); err != nil {
		return nil, err
	}
	return &f.Pipeline, nil
}

// Validate checks the pipeline definition for errors.
func (p *Pipeline) Validate() error {
	if p.Name == "" {
		return fmt.Errorf("pipeline name is required")
	}
	if p.Start == "" {
		return fmt.Errorf("pipeline start is required")
	}

	// Check start step exists
	if p.GetStep(p.Start) == nil {
		return fmt.Errorf("start step %q not found in steps", p.Start)
	}

	// Check all transition sources reference existing steps or exits
	for _, t := range p.Transitions {
		if t.Src != p.Start && p.GetStep(t.Src) == nil {
			return fmt.Errorf("transition source %q not found in steps", t.Src)
		}
		if !p.IsExit(t.Dst) && p.GetStep(t.Dst) == nil {
			return fmt.Errorf("transition destination %q not found in steps or exits", t.Dst)
		}
	}

	// Check transition conditions
	for _, t := range p.Transitions {
		if t.Condition == nil {
			continue
		}
		// Flag and Value should not both be set (unclear semantics)
		if t.Condition.Flag != "" && t.Condition.Value != "" {
			return fmt.Errorf("transition %q: condition has both flag and value set, only one allowed", t.Event)
		}
		// Count comparison fields
		cmpCount := 0
		if t.Condition.Greater != "" {
			cmpCount++
		}
		if t.Condition.Less != "" {
			cmpCount++
		}
		if t.Condition.Equal != "" {
			cmpCount++
		}
		if cmpCount > 1 {
			return fmt.Errorf("transition %q: condition has multiple comparison fields (greater/less/equal), only one allowed", t.Event)
		}
		// Greater/Less/Equal require a Value to be meaningful
		if cmpCount > 0 && t.Condition.Value == "" {
			return fmt.Errorf("transition %q: condition uses greater/less/equal but has no value field", t.Event)
		}
	}

	// Check for duplicate step names
	names := map[string]bool{}
	for _, s := range p.Steps {
		if names[s.Name] {
			return fmt.Errorf("duplicate step name: %q", s.Name)
		}
		names[s.Name] = true
	}

	// Check custom exits don't overlap with defaults (0, 1)
	for _, e := range p.Exits {
		if e.ExitCode == 0 {
			return fmt.Errorf("custom exit %q uses reserved exit code 0 (success)", e.Name)
		}
		if e.ExitCode == 1 {
			return fmt.Errorf("custom exit %q uses reserved exit code 1 (failed)", e.Name)
		}
	}

	return nil
}
