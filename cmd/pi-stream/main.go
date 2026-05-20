package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
)

type state int

const (
	idle    state = iota // no output yet
	think                // printing thinking (dim/italic)
	text                 // printing normal text
	tool                 // printing tool call/execution
)

func (s state) String() string {
	switch s {
	case idle:
		return "idle"
	case think:
		return "think"
	case text:
		return "text"
	case tool:
		return "tool"
	}
	return "?"
}

func main() {
	model := flag.String("model", "", "Model to use (e.g. go-extra/deepseek-v4-flash)")
	thinking := flag.String("thinking", "high", "Thinking level")
	tools := flag.String("t", "", "Comma-separated tool allowlist")
	session := flag.String("session", "", "pi session file path (shared between steps for context)")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: pi-stream [--model m] [--thinking l] [-t tools] <prompt>")
		os.Exit(1)
	}
	prompt := strings.Join(args, " ")

	piArgs := []string{"--mode", "rpc"}
	if *session != "" {
		piArgs = append(piArgs, "--session", *session)
	} else {
		piArgs = append(piArgs, "--no-session")
	}
	if *model != "" {
		piArgs = append(piArgs, "--model", *model)
	}
	if *thinking != "" {
		piArgs = append(piArgs, "--thinking", *thinking)
	}
	if *tools != "" {
		piArgs = append(piArgs, "-t", *tools)
	}

	cmd := exec.Command("pi", piArgs...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Fatalf("stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatalf("start pi: %v", err)
	}

	go func() {
		io.Copy(os.Stderr, stderr)
	}()

	// Send prompt
	enc := json.NewEncoder(stdin)
	if err := enc.Encode(map[string]interface{}{
		"type":    "prompt",
		"message": prompt,
	}); err != nil {
		log.Fatalf("send prompt: %v", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	st := idle
	bol := true // at beginning of line
	prevEnd := byte(0) // last byte of previous output, for space insertion

	// Print s to stdout, track line position
	emit := func(s string) {
		fmt.Print(s)
		bol = strings.HasSuffix(s, "\n")
		if len(s) > 0 {
			prevEnd = s[len(s)-1]
		}
	}

	// Add a space between tokens that don't include it themselves
	needSpace := func(nextStart byte) bool {
		if prevEnd == 0 {
			return false
		}
		// Already has whitespace on either side
		if prevEnd == ' ' || prevEnd == '\n' || prevEnd == '\t' {
			return false
		}
		if nextStart == ' ' || nextStart == '\n' || nextStart == '\t' || nextStart == 0 {
			return false
		}
		return true
	}

	// Ensure we start on a new line if we're mid-line
	ensureNewline := func() {
		if !bol {
			emit("\n")
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		typ, _ := event["type"].(string)
		switch typ {
		case "response":
			success, _ := event["success"].(bool)
			if !success {
				errMsg, _ := event["error"].(string)
				fmt.Fprintf(os.Stderr, "pi error: %s\n", errMsg)
				cmd.Process.Kill()
				os.Exit(1)
			}

		case "message_update":
			msgEvent, ok := event["assistantMessageEvent"].(map[string]interface{})
			if !ok {
				continue
			}
			dt, _ := msgEvent["type"].(string)

			switch dt {
			// ── Thinking ──
			case "thinking_start", "think_start":
				if st != idle && st != think {
					ensureNewline()
				}
				st = think

			case "thinking_delta", "thinks_delta":
				delta, _ := msgEvent["delta"].(string)
				if st != think {
					ensureNewline()
				}
				st = think
				if len(delta) > 0 && needSpace(delta[0]) {
					fmt.Print(" ")
					prevEnd = ' '
				}
				fmt.Printf("\033[2;3m%s\033[0m", delta)
				if len(delta) > 0 {
					prevEnd = delta[len(delta)-1]
				}
				bol = false

			// ── Text ──
			case "text_start":
				if st == think {
					ensureNewline()
				}
				st = text

			case "text_delta":
				delta, _ := msgEvent["delta"].(string)
				if st == think {
					ensureNewline()
				}
				st = text
				if len(delta) > 0 && needSpace(delta[0]) {
					emit(" ")
				}
				emit(delta)

			// ── Tool call (LLM decides to use a tool) ──
			case "toolcall_start":
				ensureNewline()
				tc, _ := msgEvent["toolCall"].(map[string]interface{})
				name, _ := tc["name"].(string)
				fmt.Printf("\033[1;34m🔧 %s\033[0m\n", name)
				st = tool
				bol = true

			case "toolcall_delta":
				// skip partial args

			case "toolcall_end":
				tc, _ := msgEvent["toolCall"].(map[string]interface{})
				args, _ := tc["arguments"].(map[string]interface{})
				if len(args) > 0 {
					argsJSON, _ := json.Marshal(args)
					// Use dim args on next line
					fmt.Printf("\033[2m  %s\033[0m\n", string(argsJSON))
					bol = true
				}
				st = tool
			}

		// ── Tool execution (actual running of the tool) ──
		case "tool_execution_start":
			toolName, _ := event["toolName"].(string)
			args, _ := event["args"].(map[string]interface{})
			argsJSON, _ := json.Marshal(args)
			fmt.Printf("\033[1;33m⚡ %s %s\033[0m\n", toolName, string(argsJSON))
			st = tool
			bol = true

		case "tool_execution_update":
			// streaming output, could show but skip for brevity

		case "tool_execution_end":
			toolName, _ := event["toolName"].(string)
			isErr, _ := event["isError"].(bool)
			result, _ := event["result"].(map[string]interface{})
			content, _ := result["content"].([]interface{})
			var summary string
			for _, c := range content {
				if m, ok := c.(map[string]interface{}); ok {
					if text, ok := m["text"].(string); ok {
						summary += text
					}
				}
			}
			summary = strings.TrimSpace(summary)
			if len(summary) > 200 {
				summary = summary[:200] + "..."
			}
			status := "✓"
			if isErr {
				status = "✗"
			}
			fmt.Printf("\033[2m  %s %s → %s\033[0m\n", status, toolName, summary)
			bol = true

		// ── Turn boundaries ──
		case "turn_start":
			if st != idle {
				ensureNewline()
			}
		case "turn_end":
			ensureNewline()

		// ── Completion ──
		case "agent_end":
			ensureNewline()
			stdin.Close()
			cmd.Wait()
			os.Exit(0)

		case "error":
			errMsg := ""
			if msgEvent, ok := event["assistantMessageEvent"].(map[string]interface{}); ok {
				errMsg, _ = msgEvent["error"].(string)
			}
			if errMsg == "" {
				errMsg, _ = event["error"].(string)
			}
			fmt.Fprintf(os.Stderr, "pi error: %s\n", errMsg)
			cmd.Process.Kill()
			os.Exit(1)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		os.Exit(1)
	}

	stdin.Close()
	cmd.Wait()
	os.Exit(0)
}
