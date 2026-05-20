package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lichyflow/lichyflow/internal/engine"
	"github.com/lichyflow/lichyflow/internal/pipeline"
	"github.com/lichyflow/lichyflow/internal/session"
)

var (
	pipelineFile string
	storeDir     string
	sessionID    string
	visualize    bool
	envFile      string
	envVars      []string
)

func main() {
	// Auto-load .env or .env.lichyflow from current dir (silent if missing)
	// .env.lichyflow takes precedence over .env
	_ = loadEnvFile(".env", true)
	_ = loadEnvFile(".env.lichyflow", true)

	rootCmd := &cobra.Command{
		Use:   "lichyflow",
		Short: "确定性 workflow dla agentów którzy sami nie wiedzą co robią",
		Long:  "LichyFlow — YAML-based state machine for LLM coding workflow orchestration",
		RunE:  runPipeline,
	}

	rootCmd.Flags().StringArrayVar(&envVars, "env", nil, "set env var (can be repeated: --env VAR=val)")
	rootCmd.Flags().StringVarP(&envFile, "env-file", "e", "", "path to .env file (default: .env in current dir)")
	rootCmd.Flags().StringVarP(&pipelineFile, "file", "f", "lichyflow.yaml", "pipeline definition file")
	rootCmd.Flags().StringVarP(&storeDir, "store", "s", ".lichyflow", "session store directory")
	rootCmd.Flags().StringVarP(&sessionID, "session", "i", "", "resume existing session")
	rootCmd.Flags().BoolVarP(&visualize, "visualize", "v", false, "output Mermaid diagram and exit")

	// set flag/value subcommands
	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Set a session flag or value",
	}

	setFlagCmd := &cobra.Command{
		Use:   "flag [name]",
		Short: "Set a boolean flag to true",
		Args:  cobra.ExactArgs(1),
		RunE:  runSetFlag,
	}
	setFlagCmd.Flags().StringVarP(&sessionID, "session-id", "i", "", "session ID (or set LICHYFLOW_SESSION_ID)")

	setValueCmd := &cobra.Command{
		Use:   "value [name] [value]",
		Short: "Set a string value",
		Args:  cobra.ExactArgs(2),
		RunE:  runSetValue,
	}
	setValueCmd.Flags().StringVarP(&sessionID, "session-id", "i", "", "session ID (or set LICHYFLOW_SESSION_ID)")

	setCmd.AddCommand(setFlagCmd, setValueCmd)

	// get flag/value subcommands
	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get a session flag or value",
	}

	getFlagCmd := &cobra.Command{
		Use:   "flag [name]",
		Short: "Get a boolean flag",
		Args:  cobra.ExactArgs(1),
		RunE:  runGetFlag,
	}
	getFlagCmd.Flags().StringVarP(&sessionID, "session-id", "i", "", "session ID (or set LICHYFLOW_SESSION_ID)")

	getValueCmd := &cobra.Command{
		Use:   "value [name]",
		Short: "Get a string value",
		Args:  cobra.ExactArgs(1),
		RunE:  runGetValue,
	}
	getValueCmd.Flags().StringVarP(&sessionID, "session-id", "i", "", "session ID (or set LICHYFLOW_SESSION_ID)")

	getCmd.AddCommand(getFlagCmd, getValueCmd)

	// unset flag subcommand
	unsetCmd := &cobra.Command{
		Use:   "unset",
		Short: "Unset a session flag or value",
	}

	unsetFlagCmd := &cobra.Command{
		Use:   "flag [name]",
		Short: "Unset a boolean flag (set to false)",
		Args:  cobra.ExactArgs(1),
		RunE:  runUnsetFlag,
	}
	unsetFlagCmd.Flags().StringVarP(&sessionID, "session-id", "i", "", "session ID (or set LICHYFLOW_SESSION_ID)")

	unsetCmd.AddCommand(unsetFlagCmd)

	rootCmd.AddCommand(setCmd, getCmd, unsetCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func resolveSessionID() string {
	if sessionID != "" {
		return sessionID
	}
	return os.Getenv("LICHYFLOW_SESSION_ID")
}

func resolveStoreDir() string {
	dir := storeDir
	if dir == ".lichyflow" {
		if env := os.Getenv("LICHYFLOW_STORE"); env != "" {
			dir = env
		} else {
			home, err := os.UserHomeDir()
			if err == nil {
				dir = filepath.Join(home, ".lichyflow")
			}
		}
	}
	// Expand ~ and resolve to absolute path
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, dir[2:])
		}
	}
	abs, err := filepath.Abs(dir)
	if err == nil {
		return abs
	}
	return dir
}

// loadEnvFile reads a .env file and sets each line as OS environment variable.
// Lines starting with # are comments. Empty lines are skipped.
// Format: KEY=value or export KEY=value
// If path is empty, defaults to ".env" in current directory.
// If missing is false, missing file is silently ignored.
func loadEnvFile(path string, missingOK bool) error {
	if path == "" {
		path = ".env"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && missingOK {
			return nil
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip optional "export " prefix
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Strip surrounding quotes
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if err := os.Setenv(key, val); err != nil {
			return fmt.Errorf("%s:%d: setenv %s: %w", path, i+1, key, err)
		}
	}
	return nil
}

func runPipeline(cmd *cobra.Command, args []string) error {
	// Load .env file if specified (no error if missing)
	_ = loadEnvFile(envFile, true)

	// Apply --env VAR=val overrides
	for _, e := range envVars {
		key, val, ok := strings.Cut(e, "=")
		if !ok || key == "" {
			return fmt.Errorf("invalid --env format: %q (expected VAR=val)", e)
		}
		os.Setenv(key, val)
	}

	// Resolve pipeline directory (for ci/ scripts relative to YAML)
	absPipeline, err := filepath.Abs(pipelineFile)
	if err == nil {
		os.Setenv("LICHYFLOW_PIPELINE_DIR", filepath.Dir(absPipeline))
	}

	// Load pipeline
	p, err := pipeline.LoadFromFile(pipelineFile)
	if err != nil {
		return fmt.Errorf("load pipeline: %w", err)
	}

	// Create session store (resolved to absolute path)
	storeDir := resolveStoreDir()
	os.Setenv("LICHYFLOW_STORE", storeDir)
	store := session.NewStore(storeDir)

	// Visualize mode
	if visualize {
		eng := engine.New(p, store)
		diagram, err := eng.Visualize()
		if err != nil {
			return fmt.Errorf("visualize: %w", err)
		}
		fmt.Println(diagram)
		return nil
	}

	// Create context with signal handling for clean shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run pipeline
	eng := engine.New(p, store)
	exitCode, err := eng.Run(ctx)
	if err != nil {
		return fmt.Errorf("run pipeline: %w", err)
	}

	os.Exit(exitCode)
	return nil
}

func runSetFlag(cmd *cobra.Command, args []string) error {
	sid := resolveSessionID()
	if sid == "" {
		return fmt.Errorf("session ID required: use --session-id or set LICHYFLOW_SESSION_ID")
	}
	store := session.NewStore(resolveStoreDir())
	return store.SetFlag(sid, args[0], true)
}

func runSetValue(cmd *cobra.Command, args []string) error {
	sid := resolveSessionID()
	if sid == "" {
		return fmt.Errorf("session ID required: use --session-id or set LICHYFLOW_SESSION_ID")
	}
	store := session.NewStore(resolveStoreDir())
	return store.SetValue(sid, args[0], args[1])
}

func runGetFlag(cmd *cobra.Command, args []string) error {
	sid := resolveSessionID()
	if sid == "" {
		return fmt.Errorf("session ID required: use --session-id or set LICHYFLOW_SESSION_ID")
	}
	store := session.NewStore(resolveStoreDir())
	v, err := store.GetFlag(sid, args[0])
	if err != nil {
		return err
	}
	fmt.Println(v)
	return nil
}

func runGetValue(cmd *cobra.Command, args []string) error {
	sid := resolveSessionID()
	if sid == "" {
		return fmt.Errorf("session ID required: use --session-id or set LICHYFLOW_SESSION_ID")
	}
	store := session.NewStore(resolveStoreDir())
	v, err := store.GetValue(sid, args[0])
	if err != nil {
		return err
	}
	fmt.Println(v)
	return nil
}

func runUnsetFlag(cmd *cobra.Command, args []string) error {
	sid := resolveSessionID()
	if sid == "" {
		return fmt.Errorf("session ID required: use --session-id or set LICHYFLOW_SESSION_ID")
	}
	store := session.NewStore(resolveStoreDir())
	return store.SetFlag(sid, args[0], false)
}