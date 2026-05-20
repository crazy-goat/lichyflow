# LichyFlow — Plan projektu

>确定性 workflow dla agentów którzy sami nie wiedzą co robią

## Core idea

Deterministyczny YAML-based state machine do orchestrowania LLM coding workflow.
LLM robi tylko to co wymaga inteligencji — reszta to shell scripts.
Błędy z deterministic kroków wracają do LLM jako artefakty w nowej sesji.

## Architektura

State machine ma zawsze dokładnie **jeden punkt wejścia** (`start`)
i **dwa domyślne punkty wyjścia**:
- `success` (exit code 0) — wszystko przeszło
- `failed` (exit code 1) — retry przekroczone

Dodatkowe punkty wyjścia mogą być zdefiniowane w YAML.

**Default step behavior:**
- exit code 0 → przejdź do następnego node
- exit code ≠ 0 → retry (jeśli retries left) → failed (jeśli wyczerpane)

```
                    ┌──────────┐
                    │  start   │
                    └────┬─────┘
                         │
                    ┌────▼─────┐
                    │  llm_code │◄──────────────┐
                    └────┬─────┘               │
                         │ exit 0              │ exit ≠ 0
                    ┌────▼─────┐          ┌─────▼──────┐
                    │ phpstan  │──────────►│  retry?    │
                    └────┬─────┘          └─────┬──────┘
                         │ exit 0              │
               ┌────pass┴─fail────┐      retry left?
               │                   │       │     │
          ┌────▼─────┐      ┌─────▼────┐ yes    no
          │ phpunit  │      │ llm_remedy│    ┌────▼────┐
          └────┬─────┘      └───────────┘    │ failed  │
               │                             │ exit: 1 │
          ┌────┴─────┐                       └─────────┘
          │          │
     pass ▼          ▼ fail
    ┌────────┐  ┌──────────┐
    │ success │  │ failed   │
    │ exit: 0 │  │ exit: 1  │
    └────────┘  └──────────┘
```

Default exits (zawsze obecne, nie trzeba definiować):
- `success`: exit code 0
- `failed`: exit code 1

Custom exits (opcjonalne):
- `timeout`: exit code 2
- `max_retries`: exit code 3
- dowolne inne z własnym exit code

## Kluczowe decyzje

1. **Max retries per step** — bez tego infinite loop na tokeny, default np. 3
2. **Backoff time per step** — exponential backoff między retry (default: 5s, 10s, 30s...)
3. **Timeout per step** — hard limit, zabij proces jak przekroczy (default: 300s)
4. **Nowa sesja per retry** — nie kontynuacja, czysty kontekst + artefakt + diff
5. **Artefakty overwrite** — tylko wynik z ostatniej iteracji, nie kumulacja
6. **Typed prompt per artefakt type** — phpstan-err, phpunit-err, coverage-err etc.
7. **Diff jako kontekst** — model widzi co się zmieniło, nie całą historię
8. **SESSION_ID jako główny klucz** — wszystkie artefakty, flagi, zmienne scoped do sesji
9. **CLI do mutacji sesji** — `lichyflow set value/flag xxx --session-id=yyy`
10. **Env vars przekazywane do stepów** — SESSION_ID zawsze dostępny
11. **Jeden punkt wejścia (start)** — zawsze dokładnie jeden
12. **Dwa domyślne punkty wyjścia** — `success` (exit 0) i `failed` (exit 1), zawsze obecne
13. **Default step behavior** — exit 0 = next node, exit ≠ 0 = retry, retry exhausted = failed exit
14. **Custom exits** — opcjonalne, z własnymi exit codes (timeout=2, max_retries=3, etc.)

## Research — co istnieje

- [ ] Dagu — DAG-based, ale bez conditional branching i full state machine
- [ ] GitHub Actions — za ciężki, za CI-focused
- [ ] Temporal — overkill, microservices-oriented
- [ ] Prefect/Airflow — data pipeline, nie coding workflow

## Scope MVP

### Poziom 0 — Definicja formatu

- [ ] Schema YAML dla pipeline definition
- [ ] Schema YAML dla step definition (bash, llm, condition, artifact)
- [ ] Schema YAML dla prompt templates per artefakt type
- [ ] Definicja retry/backoff/timeout per step
- [ ] Definicja env vars injection (SESSION_ID, artefact paths, custom vars)
- [ ] Definicja flag i values jako conditional transitions
- [ ] Format session store (flags, values, artefacts, retry_count)

### Poziom 1 — Runtime

- [ ] Parser YAML → state machine
- [ ] Executor bash steps (lint, test, git operations)
- [ ] Executor LLM steps (code generation, remediation)
- [ ] Artifact manager (zapisz/odczytaj wyniki między krokami)
- [ ] Session manager (nowa sesja per retry, kontekst + artefakt + diff)
- [ ] Retry logic z max_iterations

### Poziom 2 — Integracja

- [ ] Provider-agnostic LLM interface (deepseek, openai, anthropic etc.)
- [ ] Git integration (branching, commit, push, PR creation)
- [ ] CI integration (phpstan, phpunit, go test, clang-tidy etc.)

### Poziom 3 — DX

- [ ] CLI tool do uruchamiania pipeline
- [ ] Dry-run mode
- [ ] Logging i inspect (co się stało, ile tokenów, który step failnął)
- [ ] Pipeline visualization

## Learnings z eksperymentu deepseek v4 flash

- 38% tool calls to bash — workflow powinien to wziąć na siebie
- 8 tool calls na ustalenie branch name — 0 z workflow
- LLM gubi instrukcje po 50k+ tokenów — artefakty to rozwiązanie
- Mind/memory nie działało — workflow poza kontekstem jest lepszy
- Lint errors fixowane ręcznie zamiast auto-fix flag — git hooks to załatwia
- "Don't push to main" ignorowane — state machine po prostu nie pozwala

### Real-world session analysis (php-zvec BUG-005 fix)

114 messages, 55 tool calls:
- **41 bash** (75%) — git(14), gh(18), tests(4), other(5)
- **8 read** (15%) — czytanie kodu
- **5 edit** (9%) — zmiana kodu
- **1 write** (2%) — tworzenie pliku

**Deterministic steps (LichyFlow material):**
| Krok | Tool calls | LLM needed? |
|------|-----------|-------------|
| git clone/pull/reset | 3 | NIE |
| git checkout -b | 1 | NIE |
| gh issue list/view | 8 | NIE |
| php run-tests | 4 | NIE |
| git add/commit/push | 3 | NIE |
| gh pr create | 1 | NIE |
| CI polling (gh run list) | 2 | NIE |
| gh pr merge | 1 | NIE |
| gh issue close | 1 | NIE |
| git cleanup | 3 | NIE |
| **Total deterministic** | **27/55 (49%)** | **NIE** |

**LLM-required steps:**
| Krok | Tool calls | LLM needed? |
|------|-----------|-------------|
| Wybór issue | 0 (LLM reasoning) | TAK |
| Analiza kodu (read+reason) | 8 | TAK |
| Napisanie fix | 5 edit + 1 write | TAK |
| Napisanie commit msg | 0 (LLM reasoning) | TAK |
| Napisanie PR desc | 0 (LLM reasoning) | TAK |
| Code review | 1 read | TAK |

## Session & State Management

Każde uruchomienie pipeline tworzy sesję z unikalnym `SESSION_ID` (UUID).
Sesja jest jedyną jednostką izolacji — wszystko scope'uje się do niej.

### Session Store

```yaml
# .lichyflow/sessions/<session-id>/state.json
{
  "session_id": "a1b2c3d4",
  "pipeline": "php-ci",
  "current_state": "phpstan",
  "started_at": "2026-05-19T10:00:00Z",
  "retry_count": {
    "phpstan": 2,
    "phpunit": 0
  },
  "flags": {
    "phpstan_passed": false,
    "phpunit_passed": true,
    "changes_pushed": false
  },
  "values": {
    "branch_name": "fix/issue-42",
    "default_branch": "master",
    "files_changed": "4",
    "phpstan_error_count": "12"
  },
  "artifacts": {
    "phpstan-err": ".lichyflow/sessions/a1b2c3d4/phpstan-err",
    "phpunit-err": null,
    "diff": ".lichyflow/sessions/a1b2c3d4/diff.patch"
  }
}
```

### Env vars dostępne w każdym step

Każdy step (bash czy LLM) dostaje w env:

```
LICHYFLOW_SESSION_ID=a1b2c3d4
LICHYFLOW_PIPELINE=php-ci
LICHYFLOW_STATE=phpstan
LICHYFLOW_RETRY_COUNT=2
LICHYFLOW_ARTIFACT_DIR=.lichyflow/sessions/a1b2c3d4
LICHYFLOW_FLAGS_DIR=.lichyflow/sessions/a1b2c3d4/flags
LICHYFLOW_VALUES_DIR=.lichyflow/sessions/a1b2c3d4/values
```

### CLI — mutacja sesji z zewnątrz

```bash
# Set flag (boolean)
lichyflow set flag phpstan_passed --session-id=a1b2c3d4
lichyflow unset flag phpstan_passed --session-id=a1b2c3d4

# Set value (string)
lichyflow set value branch_name fix/issue-42 --session-id=a1b2c3d4
lichyflow set value phpstan_error_count 12 --session-id=a1b2c3d4

# Read z innej sesji lub z zewnątrz
lichyflow get value branch_name --session-id=a1b2c3d4
lichyflow get flag phpstan_passed --session-id=a1b2c3d4

# Wewnątrz bash step — SESSION_ID już w env
lichyflow set flag phpstan_passed       # --session-id=$LICHYFLOW_SESSION_ID implicit
```

### Conditional transitions bazują na flags/values

YAML definiuje transitions bazując na stanach i flagach:

```yaml
transitions:
  - event: phpstan_pass
    src: phpstan
    dst: phpunit
    condition:
      flag: phpstan_passed    # transition jeśli flag true
      
  - event: phpstan_fail
    src: phpstan
    dst: llm_remedy
    condition:
      flag: phpstan_passed    # transition jeśli flag false/not set
      negate: true
```

### Przykład: bash step ustawia flagę

```yaml
steps:
  - name: phpstan
    type: bash
    command: |
      phpstan analyse --error-format=raw > $LICHYFLOW_ARTIFACT_DIR/phpstan-err 2>&1
      if [ $? -eq 0 ]; then
        lichyflow set flag phpstan_passed
      else
        lichyflow unset flag phpstan_passed
      fi
    retry:
      max: 0              # phpstan sam nie retry — LLM naprawia
      timeout: 300
    timeout: 120
```

### Przykład: retry z backoff

```yaml
steps:
  - name: llm_remedy
    type: llm
    prompt_template: phpstan-remedy
    artifact_input: phpstan-err
    retry:
      max: 3
      backoff: exponential  # 5s, 10s, 30s
      initial_delay: 5s
      timeout: 600           # hard limit per attempt
```

### Punkty wejścia i wyjścia

Każdy pipeline ma dokładnie jeden punkt wejścia i dwa domyślne punkty wyjścia.
Dwa domyślne exits są ZAWSZE obecne — nie trzeba ich definiować:

```yaml
pipeline:
  name: php-ci
  
  # Punkt wejścia — dokładnie jeden
  start: llm_code
  
  # Domyślne exits — ZAWSZE obecne, nie trzeba definiować
  # exits:
  #   - name: success
  #     exit_code: 0
  #   - name: failed
  #     exit_code: 1
  
  # Custom exits — opcjonalne, jeżeli potrzebujesz więcej kodów wyjścia
  exits:
    - name: timeout
      exit_code: 2
    - name: max_retries
      exit_code: 3
```

Domyślne zachowanie każdego step:
- **exit code 0** → przejdź do następnego node
- **exit code ≠ 0** → retry (jeśli retries left) → `failed` exit (jeśli wyczerpane)

Każdy stan może transitionować do exit zamiast do innego stanu:

```yaml
transitions:
  - event: all_pass
    src: phpunit
    dst: success          # → exit code 0 (domyślny)
    
  - event: tests_fail
    src: phpunit  
    dst: failed           # → exit code 1 (domyślny)
    
  - event: retries_exhausted
    src: llm_remedy
    dst: max_retries      # → exit code 3 (custom)
```

Exit code lichyflow processa = exit code zdefiniowany w pipeline.
To pozwala np. GitLab CI schedule'ować lichyflow i reagować na różne kody wyjścia.

## Agent integration — pi jako zalecany agent

LichyFlow jest **agent-agnostic** — każdy AI agent może być użyty w stepach typu LLM.
Ale **pi** jest first-class citizen z dedykowaną integracją.

### Dlaczego pi?

- **RPC mode** (JSONL over stdin/stdout) — idealny do subprocess orchestration
- **SDK** (TypeScript) — dla deeper integration
- **Session management** — natywna obsługa sesji(branching, compaction)
- **--no-session** mode — ephermal sessions do retry loops
- **--mode json** — structured output do parsowania wyników
- **--tools** allowlist — ograniczenie tools per step(np. tylko read+edit bez bash)
- **Custom extensions** — pi extensions do custom tools
- **Provider-agnostic** — pi obsługuje anthropic, openai, google, deepseek

### Integracja LichyFlow ↔ pi

LichyFlow uruchamia pi w **RPC mode** jako subprocess:

```
LichyFlow (Go binary)
    │
    ├── bash step → bezpośrednio, env vars z session
    │
    ├── LLM step (pi) → pi --mode rpc --no-session
    │   │                     ├── stdin: JSONL commands
    │   │   └── stdout: JSONL events
    │   │
    │   └── Per-retry: nowa sesja pi → czysty kontekst
    │       ├── prompt = template + artefakt
    │       ├── tools = allowlist z YAML
    │       └── agent_end →LichyFlow czyta wynik
    │
    └── LLM step (inny agent) → interfejs
        └── provider: opencode, aider, cursor, etc.
```

### Przykład: LLM step z pi

```yaml
steps:
  - name: llm_remedy
    type: llm
    agent: pi                    # zalecany
    # agent: opencode              # alternatywny
    # agent: aider                 # alternatywny
    prompt_template: phpstan-remedy
    artifact_input: phpstan-err
    tools: [read, edit, write, bash, grep]  # allowlist
    thinking: medium
    model: deepseek/deepseek-chat  # lub anthropic/claude-sonnet-4-20250514
    retry:
      max: 3
      backoff: exponential
      initial_delay: 5s
      timeout: 600
```

LichyFlow wysyła do pi przez RPC:
```json
{"type": "prompt", "message": "<rendered prompt template z artefaktem>"}
```

I nasłuchuje events:
```json
{"type": "agent_end", "messages": [...]}
```

Na `agent_end` LichyFlow:
1. Czyta diff(`git diff` w working directory)
2. Zapisuje jako artefakt do session
3. Sprawdza czy następny step(bash) przechodzi
4. Na podstawie flag/valuestransitionuje do kolejnego stanu

### Interfejs Agent (provider-agnostic)

```go
type Agent interface {
    // Start uruchamia agent session
    Start(ctx context.Context, sessionID string, config AgentConfig) error
    
    // Prompt wysyła wiadomość do agenta
    Prompt(ctx context.Context, message string) (<-chan AgentEvent, error)
    
    // Abort przerywa aktualne działanie
    Abort(ctx context.Context) error
    
    // End kończy sesję agenta
    End(ctx context.Context) error
}

type AgentConfig struct {
    Model       string
    Tools       []string       // allowlist
    Thinking    string         // off, minimal, low, medium, high
    SystemPrompt string
    WorkDir     string
    Env         map[string]string  // SESSION_ID etc
}

type AgentEvent struct {
    Type      string      // text_delta, tool_call, tool_result, agent_end, error
    Data      interface{}
}
```

### PiAgent implementuje Agent przez RPC

```go
type PiAgent struct {
    cmd    *exec.Cmd
    stdin  io.Writer
    stdout *json.Decoder
}

func (p *PiAgent) Start(ctx context.Context, sessionID string, config AgentConfig) error {
    args := []string{"--mode", "rpc", "--no-session"}
    if config.Model != "" {
        args = append(args, "--model", config.Model)
    }
    if len(config.Tools) > 0 {
        args = append(args, "--tools", strings.Join(config.Tools, ","))
    }
    // ... start subprocess, wire stdin/stdout
}

func (p *PiAgent) Prompt(ctx context.Context, message string) (<-chan AgentEvent, error) {
    // Send JSONL command
    cmd := map[string]interface{}{"type": "prompt", "message": message}
    json.NewEncoder(p.stdin).Encode(cmd)
    
    // Read events until agent_end
    ch := make(chan AgentEvent)
    go func() {
        for {
            event := parseEvent(p.stdout)
            ch <- event
            if event.Type == "agent_end" { break }
        }
        close(ch)
    }()
    return ch, nil
}
```

Inni agenci(opencode, aider) implementują ten sam interfejs
z własnymi subprocess protocols.

## Architektura techniczna

- **Język:** Go (single binary, łatwa dystrybucja)
- **State machine core:** [looplab/fsm](https://github.com/looplab/fsm) — 3.3k⭐, aktywnie utrzymywany
  - Event-driven FSM z callbacks (before/after event, enter/leave state)
  - Wbudowany Mermaid export (stateDiagram-v2 + flowChart)
  - Wbudowany Graphviz export
  - Thread-safe z mutexami
- **Agent interface:** Go interface, pi jako first-class via RPC mode
- **To my budujemy:**
  - YAML/JSON loader → looplab/fsm
  - Conditional branching (artifact-based transitions)
  - Bash step executor
  - LLM step executor (provider-agnostic Agent interface)
  - PiAgent — pi integration via RPC mode (JSONL stdin/stdout)
  - Artifakt manager (pliki między krokami)
  - Session manager (nowa sesja per retry, czysty kontekst)
  - Prompt templates per artefakt type
  - Retry logic z max_iterations
  - Mermaid/Graphviz pipeline visualization (via looplab/fsm)
  - CLI interface

```
YAML/JSON ──► Loader ──► looplab/fsm ──► Mermaid/Graphviz
                │           │
                │     ┌─────┴──────┐
                │     │  Executor  │
                │     │  ├ bash    │
                │     │  ├ PiAgent │
                │     │  ├ OtherAgt │
                │     │  └ cond.   │
                │     └────────────┘
                │
           Artifact Manager
           Session Manager
           Prompt Templates
```

| Komponent            | looplab/fsm | LichyFlow |
|---------------------|-------------|-----------|
| State machine       | ✅          |           |
| Transitions         | ✅          |           |
| Events/triggers     | ✅          |           |
| Callbacks           | ✅          |           |
| Mermaid export      | ✅          |           |
| Graphviz export     | ✅          |           |
| YAML/JSON loader    |             | ✅        |
| Conditional branch  |             | ✅        |
| Bash executor       |             | ✅        |
| Agent interface     |             | ✅        |
| PiAgent (RPC)       |             | ✅        |
| Other agents        |             | ✅        |
| Retry logic         |             | ✅        |
| Artifacts           |             | ✅        |
| Session management  |             | ✅        |
| Prompt templates    |             | ✅        |

## Examples

### Przykład 1: Prosty flow z 3 krokami pośrednimi

```yaml
pipeline:
  name: simple-ci
  start: lint

steps:
  - name: lint
    type: bash
    command: |-
      echo "Running linter..."
      # Tu by był prawdziwy linter
      lichyflow set flag lint_passed
    timeout: 60

  - name: test
    type: bash
    command: |-
      echo "Running tests..."
      # Tu by były prawdziwe testy
      lichyflow set flag test_passed
    timeout: 120

  - name: deploy
    type: bash
    command: |-
      echo "Deploying..."
      lichyflow set flag deployed
    timeout: 300

transitions:
  - event: next
    src: lint
    dst: test

  - event: next
    src: test
    dst: deploy

  - event: next
    src: deploy
    dst: success
```

Diagram:
```
lint → test → deploy → success
```

Każdy step domyślnie: exit 0 = next, exit ≠ 0 = retry/failed.
Flow liniowy, bez conditionals — proste przejście krok po kroku.

### Przykład 2: Retry z variable increment

```yaml
pipeline:
  name: retry-counter
  start: init

steps:
  - name: init
    type: bash
    command: lichyflow set value counter 0
    # init zawsze przechodzi (exit 0) → next

  - name: check
    type: bash
    command: |-
      # Pobierz aktualną wartość i zwiększ o 1
      current=$(lichyflow get value counter)
      new=$((current + 1))
      lichyflow set value counter $new
      echo "counter: $new"

      # Sprawdź warunek — przejdź dalej jeśli > 2
      if [ $new -gt 2 ]; then
        echo "counter > 2, passing!"
        exit 0
      else
        echo "counter <= 2, will retry ($new/3)"
        exit 1
      fi
    retry:
      max: 3
      backoff: fixed
      initial_delay: 1s
    timeout: 10

transitions:
  - event: next
    src: init
    dst: check

  - event: next
    src: check
    dst: success
```

Diagram:
```
init → check (retry) → success
        │
        ├── run 1: counter 0→1, 1 > 2? NO → fail → retry
        ├── run 2: counter 1→2, 2 > 2? NO → fail → retry  
        └── run 3: counter 2→3, 3 > 2? YES → exit 0 → success
```

Ten przykład pokazuje:
- Zmienne sesji (`counter`) które przetrwają między retry
- Retry z max=3 i backoff
- Warunek exit zależny od wartości zmiennej
- Ostatecznie success po 3 próbach

## Open questions

- [x] Jaki LLM provider jako default? → pi jest agent-em, wybór provider to konfiguracja pi
- [ ] Czy pipeline jest per-repo czy per-task?
- [ ] Jak reprezentować diff między iteracjami?
- [ ] Czy artefakty to pliki w repo czy w external storage?
- [ ] Format loader: tylko YAML czy YAML + JSON?

## Inspiracje

- [looplab/fsm](https://github.com/looplab/fsm) — Go FSM core z Mermaid/Graphviz export
- Dagu (https://github.com/dagucloud/dagu) — DAG executor
- GitHub Actions — expression syntax, artifact model
- Temporal — state machine patterns
- Git hooks — deterministic enforcement