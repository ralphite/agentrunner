# AgentRunner — High-Level Design

A flexible agent runner/harness. Prototype-quality implementation: clean design, zero
legacy, no backward-compat obligations. This document is the living design record —
it grows as we discuss and add items.

## Goals

- Run one or many LLM agents defined entirely by declarative specs.
- Make every run durable, inspectable, and replayable.
- Keep the core small: a handful of orthogonal primitives that compose.

## Non-goals (for the prototype)

- Distributed/multi-node execution (design leaves the door open; we build single-process).
- Backward compatibility of specs, events, or APIs between iterations.
- Production hardening (auth, multi-tenancy, quotas across users).

---

## Core primitives

The system is built from six primitives. Everything else is composition.

```
┌─────────────────────────────────────────────────────────────┐
│                          Runtime                            │
│                                                             │
│  ┌───────────┐   ┌───────────┐   ┌───────────┐              │
│  │  Actor A  │   │  Actor B  │   │  Actor C  │  ...         │
│  │ (agent)   │   │ (agent)   │   │ (workflow)│              │
│  └─────┬─────┘   └─────┬─────┘   └─────┬─────┘              │
│        │  mailboxes    │               │                    │
│  ══════╧═══════════════╧═══════════════╧══════ Message Bus  │
│        │                                                    │
│  ┌─────┴──────────┐      ┌────────────────┐                 │
│  │  Event Store   │◄─────┤  Checkpoints   │                 │
│  │ (append-only)  │      │  (snapshots)   │                 │
│  └────────────────┘      └────────────────┘                 │
└─────────────────────────────────────────────────────────────┘
```

### 1. Actor model

- Every runnable thing is an **actor**: an id, a mailbox, and a behavior.
  Agents, workflows, and system services (journal, scheduler) are all actors.
- Actors process one message at a time from their mailbox — no shared mutable
  state, no locks. Concurrency comes from having many actors.
- Actors can `spawn` children and `send` messages to any actor by id.
- **Supervision**: every actor has a parent. A crashed actor notifies its
  supervisor, which applies a restart policy (`restart` from last checkpoint,
  `resume`, `stop`, `escalate`).

### 2. Message bus

- Single in-process bus with two delivery modes:
  - **send(to, msg)** — point-to-point, goes to one actor's mailbox.
  - **publish(topic, msg)** — pub/sub fan-out to all subscribers.
- All messages are immutable **envelopes**:

  ```
  Envelope {
    id            # unique message id
    causation_id  # message that caused this one
    correlation_id# groups a whole conversation/run
    sender, target (actor id or topic)
    type, payload
    ts
  }
  ```

- Causation/correlation ids give us free distributed-tracing-style lineage
  through the event log.
- The bus is transport only — it is ephemeral. Persistence happens in the
  event store (next section), not in the bus.

### 3. Event sourcing

- Two vocabularies, strictly separated:
  - **Commands** — intents, flow over the bus (`RunAgent`, `CallTool`, `CancelRun`).
  - **Events** — immutable facts, appended to the store (`AgentStarted`,
    `LlmCalled`, `ToolReturned`, `RunCompleted`).
- An actor's state is never mutated directly. The only path is:
  `handle(command) → emit(events) → append to store → apply(event) to state`.
- The event store is append-only, partitioned per actor (a *stream*), with a
  monotonically increasing sequence number per stream.
- Prototype backend: JSONL file per stream under `runs/<run_id>/`. The store is
  behind a small interface so SQLite (or anything) can replace it later.
- The full event log **is** the audit trail, the debugger, and the test
  fixture: any run can be re-derived from its events.

### 4. Checkpoints

- Replaying thousands of events to recover an actor is wasteful. A
  **checkpoint** is a snapshot of actor state at stream sequence `N`.
- Recovery = load latest snapshot + replay events with `seq > N`.
- Checkpoint triggers: every K events, and at workflow step boundaries.
- Snapshots are disposable — deleting them only costs replay time, never
  correctness. The event log remains the source of truth.

### 5. Durable workflow

- A workflow is deterministic orchestration code whose **side effects are
  recorded** (Temporal-style):
  - Effectful operations (LLM call, tool call, MCP call, sleep, sub-agent
    invocation) run as **activities**.
  - Each activity's result is persisted as an event before the workflow
    proceeds.
  - On replay after a crash/restart, activities whose results are already in
    the log return the recorded result instead of re-executing.
- Consequence: a run survives process death mid-flight. Restart the runner and
  every in-progress run resumes at the exact step it left off — without
  re-calling the LLM for completed steps.
- The **agent loop itself is a durable workflow**: each LLM turn and each tool
  execution is an activity. Nothing special is needed to make agents durable.
- Workflow code must be deterministic: no wall-clock reads, no RNG, no I/O
  outside activities. The runtime enforces this by construction (workflows only
  get an `ctx` handle that exposes activities, timers, and messaging).

### 6. Agent spec

Agents are defined entirely by declarative specs (YAML). Everything is
configurable; nothing is hard-coded in the runner.

```yaml
# agents/researcher.yaml
name: researcher
description: Deep-dives a topic and reports findings.

model:
  provider: anthropic
  id: claude-sonnet-5
  max_tokens: 8192

system_prompt: |
  You are a meticulous researcher...
  # or: system_prompt_file: prompts/researcher.md

tools:                      # built-in tool allowlist
  - read_file
  - web_search

mcp:                        # MCP servers this agent may use
  - name: github
    transport: stdio
    command: ["github-mcp-server"]
    allowed_tools: [search_code, get_file_contents]   # optional narrowing

skills:                     # directories of markdown skills, loaded on demand
  - ./skills/research

agents:                     # sub-agents this agent may spawn (by spec name)
  - summarizer

limits:
  max_turns: 40
  max_tokens_total: 500_000
  timeout_s: 900
```

- Specs are validated into typed models (pydantic) at load time; a bad spec
  fails fast with a precise error.
- A spec is a *template*; an **agent instance** is an actor created from a spec
  plus runtime input (the task, correlation id, parent).
- Prompts, tools, MCP servers, skills, models, and limits are all data. Adding
  a new agent means adding a YAML file, not code.

### 7. Multi-agent

- Because agents are actors on a shared bus, multi-agent is not a special
  subsystem — it's actors sending messages. The runner provides patterns on top:
  - **Spawn/await** — an agent invokes a sub-agent as an activity and awaits
    its result (fan-out with N children works the same way).
  - **Handoff** — an agent transfers the conversation/task to another agent and
    exits.
  - **Pub/sub collaboration** — agents subscribe to topics (e.g. a blackboard
    topic) and react to each other's findings.
- Sub-agent results flow through the parent's event log like any activity, so
  a multi-agent run replays deterministically as a whole tree.
- The spec's `agents:` list is an allowlist — an agent can only spawn what its
  spec permits.

---

## Key design decisions

| # | Decision | Choice | Rationale |
|---|----------|--------|-----------|
| 1 | Language | Python 3.12+, asyncio | Async actors map cleanly onto tasks + queues; pydantic for specs; mature MCP + Anthropic SDKs. |
| 2 | Process model | Single process, in-memory bus | Prototype simplicity. Actor + event-sourcing boundaries mean distribution later is a transport swap, not a redesign. |
| 3 | Bus vs. store | Bus is ephemeral transport; event store is the only persistence | Avoids the "is the bus durable?" tarpit. Durability lives in exactly one place. |
| 4 | Commands vs. events | Strictly separated types | Keeps intent (retryable, rejectable) distinct from fact (immutable, replayable). |
| 5 | Storage backend | JSONL per stream, behind an `EventStore` interface | Human-readable runs, trivially diffable; swap to SQLite when needed. |
| 6 | Durability model | Temporal-style record/replay of activities | Simplest model that makes the *agent loop itself* crash-safe and resumable. |
| 7 | Spec format | YAML → pydantic models | Declarative, reviewable, no code needed per agent. |

## Repository layout (proposed)

```
agentrunner/
  core/
    actor.py        # Actor, Mailbox, Supervisor, spawn/lifecycle
    bus.py          # MessageBus: send/publish/subscribe, Envelope
    events.py       # Command/Event base types, envelope schema
    store.py        # EventStore interface + JSONL backend
    checkpoint.py   # Snapshot save/load, recovery
    workflow.py     # Durable workflow engine: activities, replay, WorkflowCtx
  agent/
    spec.py         # AgentSpec pydantic models + loader
    agent.py        # The agent loop as a durable workflow
    tools.py        # Built-in tool registry
    mcp.py          # MCP client lifecycle per spec
    skills.py       # Skill discovery/loading
  runtime.py        # Runtime: wires bus + store + root supervisor, run entrypoint
  cli.py            # `agentrunner run <spec> "task"`, `agentrunner replay <run_id>`
agents/             # example agent specs
runs/               # event logs + checkpoints (gitignored)
tests/
DESIGN.md
```

## Proposed additions (to discuss)

Items not in the initial list that fall out naturally from this design:

1. **Observability** — the event log is already a trace; add a `replay`/`inspect`
   CLI that renders a run as a timeline (turns, tool calls, sub-agents, tokens).
2. **Human-in-the-loop** — approvals as first-class: an agent emits an
   `ApprovalRequested` event and the workflow durably parks until an
   `ApprovalGranted` command arrives (minutes or days later — durability makes
   this free).
3. **Budgets & limits enforcement** — the runtime (not the agent) enforces
   `limits:` from the spec, emitting `LimitExceeded` and stopping the actor.
4. **Triggers/scheduling** — cron- or event-triggered runs; a scheduler is just
   another actor publishing `RunAgent` commands.
5. **Deterministic testing** — record a run once, then replay it in tests with
   stubbed activities; agent behavior changes show up as event-log diffs.
6. **Context/memory management** — compaction, summarization, and cross-run
   memory as configurable spec sections.

## Open questions

- **LLM provider abstraction**: Anthropic-only for the prototype, or a thin
  provider interface from day one? (Leaning: thin interface, one implementation.)
- **Skill format**: adopt the Claude Code skill convention (directory of
  markdown with frontmatter) or define our own minimal format?
- **Streaming**: do we surface token streaming to the CLI in v0, or is
  turn-granularity output enough for the prototype?
- **Event schema versioning**: none for the prototype (re-run instead of
  migrate) — confirm we're fine discarding old run logs on schema changes.

## Roadmap

1. **M1 — Kernel**: actors, bus, event store, checkpoints; toy actor test.
2. **M2 — Durable workflows**: activity record/replay, crash-resume test.
3. **M3 — Single agent**: spec loader, agent loop as workflow, built-in tools, CLI.
4. **M4 — MCP + skills**: MCP server lifecycle, skill loading.
5. **M5 — Multi-agent**: spawn/await, handoff, pub/sub patterns; example fleet.
