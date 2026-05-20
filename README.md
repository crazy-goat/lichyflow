# LichyFlow

确定性 workflow dla agentów którzy sami nie wiedzą co robią

YAML-based state machine for LLM coding workflow orchestration.

## Build

```bash
make build
```

Binary: `./lichyflow`

## Install

```bash
make install
```

Installs to `/usr/local/bin/lichyflow`.

## Test

```bash
make test
```

## Run examples

```bash
make examples
```

## Visualize (Mermaid)

```bash
make visualize
```

## Quick start

```bash
# Build
make build

# Run a pipeline
./lichyflow -f examples/01-simple-ci.yaml -s /tmp/lichyflow-data

# Run with output (needs lichyflow in PATH)
export PATH="$(pwd):$PATH"
./lichyflow -f examples/02-retry-counter.yaml -s /tmp/lichyflow-data

# Visualize pipeline as Mermaid diagram
./lichyflow -v -f examples/02-retry-counter.yaml

# Set/get session values (from within bash steps or externally)
lichyflow set value counter 5 --session-id=abc123 -s /tmp/lichyflow-data
lichyflow get value counter --session-id=abc123 -s /tmp/lichyflow-data
lichyflow set flag phpstan_passed --session-id=abc123 -s /tmp/lichyflow-data
lichyflow unset flag phpstan_passed --session-id=abc123 -s /tmp/lichyflow-data
```

## Architecture

```
YAML/JSON ──► Loader ──► looplab/fsm ──► Mermaid/Graphviz
                │           │
                │     ┌─────┴──────┐
                │     │  Executor  │
                │     │  ├ bash    │
                │     │  ├ PiAgent │
                │     │  └ cond.   │
                │     └────────────┘
                │
           Artifact Manager
           Session Manager
           Prompt Templates
```

See [PLAN.md](PLAN.md) for full design docs.# test
