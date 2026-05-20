package pipeline

// Requirement defines a required command/tool that must exist on the system.
type Requirement struct {
	Name    string `yaml:"name,omitempty" json:"name,omitempty"`     // friendly name for logs
	Command string `yaml:"command,omitempty" json:"command,omitempty"` // command name (checked via "which")
	Check   string `yaml:"check,omitempty" json:"check,omitempty"`     // bash command that must exit 0
	Hint    string `yaml:"hint,omitempty" json:"hint,omitempty"`       // optional help text if missing
}

// Pipeline represents a complete workflow definition.
type Pipeline struct {
	Name         string        `yaml:"name" json:"name"`
	Start        string        `yaml:"start" json:"start"`
	Requirements []Requirement `yaml:"requirements,omitempty" json:"requirements,omitempty"`
	Exits        []Exit        `yaml:"exits" json:"exits,omitempty"`
	Steps        []Step        `yaml:"steps" json:"steps"`
	Transitions  []Transition  `yaml:"transitions" json:"transitions"`
}

// Step represents a single step in the pipeline.
type Step struct {
	Name           string     `yaml:"name" json:"name"`
	Type           string     `yaml:"type" json:"type"` // "bash" or "llm"
	Command        string     `yaml:"command,omitempty" json:"command,omitempty"`           // bash command
	PromptTemplate string     `yaml:"prompt_template,omitempty" json:"prompt_template,omitempty"` // llm prompt template
	ArtifactInput  string     `yaml:"artifact_input,omitempty" json:"artifact_input,omitempty"`
	Agent          string     `yaml:"agent,omitempty" json:"agent,omitempty"`     // "pi" (default), "opencode", etc.
	Model          string     `yaml:"model,omitempty" json:"model,omitempty"`
	Tools          []string   `yaml:"tools,omitempty" json:"tools,omitempty"`
	Thinking       string     `yaml:"thinking,omitempty" json:"thinking,omitempty"` // off, minimal, low, medium, high
	Env            map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Retry          RetryConfig `yaml:"retry,omitempty" json:"retry,omitempty"`
	Timeout        int        `yaml:"timeout,omitempty" json:"timeout,omitempty"` // seconds, default 300
}

// RetryConfig defines retry behavior for a step.
type RetryConfig struct {
	Max           int    `yaml:"max" json:"max"`                      // max retries, default 0
	Backoff       string `yaml:"backoff,omitempty" json:"backoff,omitempty"` // "fixed" or "exponential", default "exponential"
	InitialDelay  string `yaml:"initial_delay,omitempty" json:"initial_delay,omitempty"` // e.g. "5s", default "5s"
	Timeout       int    `yaml:"timeout,omitempty" json:"timeout,omitempty"` // per-attempt timeout in seconds
}

// Transition defines a state transition triggered by an event.
type Transition struct {
	Event     string      `yaml:"event" json:"event"`
	Src       string      `yaml:"src" json:"src"`
	Dst       string      `yaml:"dst" json:"dst"`
	Condition *Condition  `yaml:"condition,omitempty" json:"condition,omitempty"`
}

// Condition defines a conditional transition based on session flags/values.
type Condition struct {
	Flag    string `yaml:"flag,omitempty" json:"flag,omitempty"`
	Value   string `yaml:"value,omitempty" json:"value,omitempty"`
	Greater string `yaml:"greater,omitempty" json:"greater,omitempty"` // value > this
	Less    string `yaml:"less,omitempty" json:"less,omitempty"`       // value < this
	Equal   string `yaml:"equal,omitempty" json:"equal,omitempty"`     // value == this
	Negate  bool   `yaml:"negate,omitempty" json:"negate,omitempty"`
}

// Exit defines a terminal state with an exit code.
type Exit struct {
	Name     string `yaml:"name" json:"name"`
	ExitCode int    `yaml:"exit_code" json:"exit_code"`
}

// DefaultExits returns the two always-present default exits.
func DefaultExits() []Exit {
	return []Exit{
		{Name: "success", ExitCode: 0},
		{Name: "failed", ExitCode: 1},
	}
}

// AllExits returns default exits merged with custom exits.
func (p *Pipeline) AllExits() []Exit {
	exits := DefaultExits()
	customCodes := map[int]bool{0: true, 1: true}
	for _, e := range p.Exits {
		if !customCodes[e.ExitCode] {
			exits = append(exits, e)
			customCodes[e.ExitCode] = true
		}
	}
	return exits
}

// IsExit checks if a state name is an exit state.
func (p *Pipeline) IsExit(name string) bool {
	for _, e := range p.AllExits() {
		if e.Name == name {
			return true
		}
	}
	return false
}

// GetExitCode returns the exit code for an exit state name.
func (p *Pipeline) GetExitCode(name string) int {
	for _, e := range p.AllExits() {
		if e.Name == name {
			return e.ExitCode
		}
	}
	return 1 // default to failed
}

// GetStep returns a step by name.
func (p *Pipeline) GetStep(name string) *Step {
	for i := range p.Steps {
		if p.Steps[i].Name == name {
			return &p.Steps[i]
		}
	}
	return nil
}