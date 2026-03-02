# AGENTS.md

## Purpose And Principles

This file defines the operating system for project execution across any software repo.

Principles:

- ship in small, reviewable increments
- keep work traceable from idea to release
- enforce quality gates before merge
- keep docs and code synchronized
- optimize for sustainability, not heroics
- enforce DRY: avoid duplicated logic, validation rules, and output formatting paths

## Roles

Use these roles even on a one-person team (one person may hold multiple roles):

- `Requester`: defines outcome and business context
- `Owner`: accountable for ticket delivery
- `Reviewer`: validates correctness, risk, and maintainability
- `Release Steward`: handles release notes, tagging, and rollback readiness

## Ticket System

Every non-trivial change needs a ticket.

Repository implementation:

- ticket files live under `tickets/<status>/`
- generated board is `tickets/board.md`
- create ticket via `./scripts/tickets/new.sh`
- move ticket via `./scripts/tickets/move.sh`
- rebuild board via `./scripts/tickets/board.sh`

Required fields:

- `ID`: `PROJ-YYYY-NNN` (example: `CORE-2026-014`)
- `Title`: action-oriented, specific
- `Priority`: `P0`, `P1`, `P2`, `P3`
- `Status`: `Backlog`, `Ready`, `In Progress`, `Review`, `Blocked`, `Done`, `Archived`
- `Owner`: single accountable person
- `Due Date`: ISO date (`YYYY-MM-DD`) or `None`
- `Acceptance Criteria`: measurable checklist
- `Links`: design doc, PR, incident, related tickets

Rules:

- no ticket ID, no merge (except emergency hotfix with retroactive ticket)
- each PR references exactly one primary ticket
- status changes must include a short note
- when implementing ticket scope, immediately mark completed acceptance/test checklist points as done (`[x]`) in the same change set
- do not leave completed ticket points unchecked; keep ticket progress state accurate at all times

## Current State In AGENTS (MANDATORY)

`AGENTS.md` is the canonical current-state snapshot of the system.

Interpretation rules:

- tickets are change history (event stream): they describe what changed and why
- `AGENTS.md` captures the materialized current behavior of the system
- when behavior/contracts/command UX/storage/security boundaries change, update current-state sections in `AGENTS.md` in the same change set
- if ticket text and `AGENTS.md` differ, `AGENTS.md` is source of truth for current behavior
- keep current-state sections concise, readable, and human-oriented (small sections, explicit contracts, minimal prose)

## Current System State

### Auth Subcommands

Current model:

- `wbcli auth` is single-session only (`logged in` or `logged out`)
- no profile model in auth flow

Available commands:

- `wbcli auth login`
- `wbcli auth logout`
- `wbcli auth status`

Removed commands:

- `wbcli auth set`
- `wbcli auth use`
- `wbcli auth list`
- `wbcli auth current`
- `wbcli auth test`

`auth login` contract:

- stdin-only credential input
- exactly two non-empty logical lines:
  - line 1 = API key
  - line 2 = API secret
- max payload size: `16 KiB`
- no `--api-key`, no `--api-secret`, no `--profile`
- login performs signed connectivity validation with WhiteBIT via:
  - `POST /api/v4/collateral-account/hedge-mode`
  - request body includes `request` + monotonic `nonce`
  - probe response hedge mode is written to session metadata config
  - credentials are persisted only when probe succeeds

Outputs:

- `auth login`: `logged_in=true backend=<backend> api_key=<masked_hint> saved_at=<timestamp>`
- `auth logout`: `logged_out=true`
- `auth status`:
  - `logged_in=false`
  - or `logged_in=true backend=<backend> api_key=<masked_hint> updated_at=<timestamp>`

Storage/security boundaries:

- secrets are stored in `os-keychain` backend only
- non-secret metadata is stored in `~/.wbcli/config.yaml` as YAML
- runtime config reader must support legacy JSON content for backward compatibility; writer persists YAML only
- session metadata includes cached `hedge_mode` from WhiteBIT probe
- config permission target on macOS/Linux: `0600`
- command outputs/errors must not leak API secret, payload, or signature values

### Collateral Order Commands

Current model:

- single-order placement command path is `wbcli collateral order place`
- legacy root path `wbcli order ...` is removed
- `collateral order place` uses single-session auth credentials from keychain-backed store

`collateral order place` contract:

- required flags:
  - `--market`
  - `--side`
  - `--amount`
  - `--price`
- optional flags:
  - `--client-order-id` (pass-through only)
  - `--output table|json` (default `table`)
- accepted side values are: `buy`, `sell`, `long`, `short`
- command always submits `postOnly=true`
- no `--profile`, no `--expiration`
- request mapping is hedge-mode-aware:
  - hedge mode `true`: `long|buy` -> `side=buy, positionSide=long`; `short|sell` -> `side=sell, positionSide=short`
  - hedge mode `false`: `positionSide` is omitted and side is normalized to `buy|sell`
- if order submission fails with WhiteBIT hedge-mode mismatch (`hedgeMode: Order's position side does not match user's setting`), service refreshes hedge mode via `/api/v4/collateral-account/hedge-mode`, persists it in config, and retries once

Output contract:

- `request_id`
- `mode`
- `orders_planned`
- `orders_submitted`
- `orders_failed`
- `errors[]`

### WhiteBIT Transport Client

Current adapter behavior (`internal/adapters/whitebit`):

- one shared signed private client is the source of truth for WhiteBIT HTTP calls
- implemented private endpoints:
  - `POST /api/v4/collateral-account/hedge-mode`
  - `POST /api/v4/order/collateral/limit`
  - `POST /api/v4/order/collateral/bulk`
- auth login verification is implemented through a dedicated credential-verifier adapter that calls the client hedge-mode endpoint
- request signing is centralized and reused for all private calls (`X-TXC-APIKEY`, `X-TXC-PAYLOAD`, `X-TXC-SIGNATURE`)
- golden rule: client mirrors API documentation only (endpoints/fields/errors), nothing more and nothing less
  - do not add business methods like `Verify` to the transport client
  - business orchestration belongs to application services (for example `LoginService`) and their outbound adapters

### Application Factory

Current wiring model:

- command groups use one shared application container per CLI process (`internal/app/application`)
- root command creates a cached application provider and passes it into command groups
- application container exposes use-case interfaces (for example `AuthUseCases`) rather than direct adapter dependencies
- tests override runtime wiring via `cmd.SetApplicationFactoryForTest`

## Backlog And Todo Workflow

### Triage

- triage new items at least 2 times per week
- reject or archive vague items without clear value
- convert ideas into tickets only when problem + outcome are explicit

### Planning

- maintain a `Ready` queue with top priorities for next 1-2 weeks
- assign owner before moving to `In Progress`
- split tickets estimated over 2 days of focused work

### WIP Limits

- max `In Progress` per owner: `2`
- no new start while blocked ticket lacks unblock action

### Archive Rules

- move done tickets to `Archived` after 30 days
- close stale backlog items after 60 days without activity

## Definition Of Ready (DoR)

A ticket is `Ready` only if:

- problem statement is clear
- acceptance criteria are testable
- dependencies are identified
- rollout and rollback notes exist (for risky changes)
- owner and priority are set

## Definition Of Done (DoD)

A ticket is `Done` only if:

- code merged to main branch
- required tests pass
- docs/changelog updated
- monitoring/alerts considered (if production-impacting)
- acceptance criteria verified

## Branching And Commit Conventions

Team model:

- team size: 2 developers
- trunk-based development: both developers commit directly to `main`
- keep commits small and focused to reduce merge friction

Commit format:

- `<type>(<scope>): <short summary>`
- types: `feat`, `fix`, `chore`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`
- include ticket ID in commit body or footer (`Refs: PROJ-2026-014`)

Rules:

- commits must be very small and truly atomic (one clear intent per commit)
- split large tasks aggressively: for example, `auth set/list/remove/test` should be split not only by command but also by command parts (flags/input, validation, storage adapter wiring, tests, docs)
- before work, run `git pull --rebase origin main`
- after work and before push, run `git pull --rebase origin main`
- if pull introduces conflicts, resolve them locally, re-run required checks, then push
- push only when local `main` is up to date with `origin/main`
- always push immediately after each commit (do not batch local commits)

## Change Checklist And Review Standards

For trunk-based direct commits to `main`, each change set (commit or small commit series) must include:

- linked ticket
- what changed and why
- risk level (`low`, `medium`, `high`)
- test evidence
- rollback plan (if needed)

Reviewer checks:

- correctness and edge cases
- security/privacy impact
- migration compatibility
- test quality (not only coverage)
- docs consistency

Merge policy:

- prefer 1 peer review before push when practical; if not practical, request review immediately after push
- all required CI checks must pass on `main`
- if CI fails after push, prioritize immediate fix or revert

## Testing Expectations By Change Type

- `docs-only`: spelling/link checks
- `ui/ux`: component tests + visual/manual sanity
- `api/logic`: unit + integration tests for success/failure paths
- `data/schema`: migration test + backward compatibility check
- `infra/ci`: dry-run in non-prod path where possible
- `hotfix`: minimal reproducer test added within 24 hours post-fix

## Project Readiness Quality Gate (MANDATORY)

Before considering a change ready, run these checks when relevant to the change:

- `gofmt -w` on changed Go files
- `go vet ./...`
- `go test ./...`
- `go build .`

If any step fails, the change is not ready and must be fixed before merge/push.

## CLI Installability Requirement

VERY IMPORTANT (MANDATORY):

- CLI must remain installable directly from repository root using:
  - `go install github.com/ChewX3D/wbcli@latest`
- changes must not break root package build/install flow
- entrypoint and wiring for installable CLI must stay rooted in top-level `main.go`

Required verification for CLI-impacting changes:

- run `mkdir -p /tmp/gobin`
- run `go build -o /tmp/gobin .`
- run `go test ./...`
- run `go install github.com/ChewX3D/wbcli@latest` (or equivalent local validation in restricted environments)

## Go Code Style (Google)

VERY IMPORTANT (MANDATORY):

- all Go code must follow Google Go style guidance
- style violations are blocking issues and must be fixed before merge

For all Go code in this repository, follow the Google Go style guides in full:

- Go Style Guide: https://google.github.io/styleguide/go/
- Style Decisions: https://google.github.io/styleguide/go/decisions
- Best Practices: https://google.github.io/styleguide/go/best-practices

These references are the source of truth. If this repo guidance and Google guidance differ, Google Go guidance wins unless a repo-specific exception is explicitly documented.

Required implementation rules for day-to-day work:

- Formatting:
  - run `gofmt` on changed Go files before commit
  - keep imports gofmt-compatible and clean (no unused imports)
  - keep package layout simple and cohesive; one package responsibility per directory
- Naming and declarations:
  - use idiomatic Go names (`MixedCaps`, no underscores in identifiers)
  - use clear package names; avoid stuttered APIs (`foo.FooType`)
  - keep exported identifiers documented with comments that start with the identifier name
  - use consistent initialisms (`ID`, `HTTP`, `URL`, `JSON`)
- Error handling:
  - check and handle every returned error; do not ignore errors silently
  - wrap errors with context using `%w` when propagating (`fmt.Errorf("...: %w", err)`)
  - keep error strings lowercase and without trailing punctuation
  - reserve `panic` for truly unrecoverable programmer/runtime faults, not normal control flow
- Context and API shape:
  - pass `context.Context` as the first argument (`ctx context.Context`) when cancellation/deadlines are relevant
  - do not store `Context` in structs
  - accept interfaces where useful, but return concrete types from constructors/functions
  - design zero-value-safe types where practical
- Concurrency:
  - make goroutine ownership/lifecycle explicit and prevent leaks
  - close channels only from the sender side; document channel ownership
  - protect shared mutable state with synchronization and avoid data races
- Tests:
  - prefer table-driven tests for multi-case logic
  - make failures actionable (`got` vs `want` in error messages)
  - keep tests deterministic; avoid time-based flakiness
  - include coverage for success paths, error paths, and edge cases
- Documentation:
  - do not create `doc.go` files
  - if package-level comments are needed, place them in regular source files (for example `types.go`, `service.go`, `client.go`)
  - document exported behavior, invariants, side effects, and concurrency expectations
  - keep examples/docs synchronized with current CLI/API behavior
- DRY enforcement:
  - do not duplicate business logic across commands/services/adapters
  - extract shared behavior into small helpers when the same pattern appears 2+ times
  - keep one source of truth for validation and error mapping rules
  - in reviews, treat avoidable duplication as a blocking issue unless explicitly justified

Authoritative DRY references (MANDATORY):

- Original DRY framing by Dave Thomas and Andy Hunt:
  - https://artima.com/intv/dry.html
- Martin Fowler on code smells (duplicated code indicates deeper design issues):
  - https://martinfowler.com/bliki/CodeSmell.html
- Refactoring discipline for safe, incremental deduplication:
  - https://refactoring.com/

Rules derived from these references:

- DRY is about duplicate knowledge/behavior, not blind token-level deduplication
- remove duplication using small, tested refactors; avoid broad rewrites without safety nets
- if duplicated knowledge is intentional (for isolation or risk control), document why duplication is acceptable

## Go Deep Knowledge (Official go.dev)

VERY IMPORTANT (MANDATORY):

- these official Go references are required engineering context for this repository
- when introducing complex behavior (concurrency, performance, memory, tooling, security), align implementation and reviews with these sources
- if there is ambiguity, prefer the official go.dev references below over informal blog/forum guidance

Primary references:

- Language specification:
  - https://go.dev/ref/spec
- Memory model:
  - https://go.dev/ref/mem
- Data race detector:
  - https://go.dev/doc/articles/race_detector
- Diagnostics (profiles, traces, runtime stats, debugging):
  - https://go.dev/doc/diagnostics
- Garbage collector guide (cost model, escape analysis, tuning):
  - https://go.dev/doc/gc-guide
- Native fuzzing:
  - https://go.dev/doc/security/fuzz
- Module reference:
  - https://go.dev/ref/mod
- Dependency management:
  - https://go.dev/doc/modules/managing-dependencies
- Vulnerability management (`govulncheck`, vuln DB):
  - https://go.dev/doc/security/vuln/
- Release notes policy and version awareness:
  - https://go.dev/doc/devel/release
  - https://go.dev/doc/go1.25
  - https://go.dev/doc/go1.26
- Concurrency and cancellation patterns:
  - https://go.dev/blog/context
  - https://go.dev/blog/context-and-structs
  - https://go.dev/blog/pipelines

Official WhiteBIT reference (for signing/auth flow):

- https://github.com/whitebit-exchange/api-quickstart/blob/master/src/go/auth.go
  - can be used as an official WhiteBIT reference implementation

Mandatory engineering rules derived from these docs:

- Concurrency correctness:
  - any shared mutable state across goroutines must have explicit synchronization
  - no intentional data races; `go test -race ./...` is required for concurrent logic changes
  - design channel ownership/closure rules up front and document them in package-level comments
- Context discipline:
  - pass `context.Context` as first parameter when cancellation/deadlines are relevant
  - do not store context in structs except rare, explicitly justified boundary cases
  - use cancellation in fan-out/fan-in pipelines to prevent goroutine leaks
- Memory and GC literacy:
  - treat heap growth as a measurable budget, not a surprise side effect
  - investigate allocations with profiles first; then inspect escapes with:
    - `go build -gcflags=-m=3 <package>`
  - tune `GOGC` and `GOMEMLIMIT` only with benchmark/profile evidence and documented rationale
- Diagnostics-first performance work:
  - no performance claims without data from profiles/traces/benchmarks
  - prefer repeatable workflows (`go test -bench`, `pprof`, runtime trace) and commit evidence in PR notes
- Testing beyond happy path:
  - add fuzz tests for parser/normalizer/validation code paths exposed to untrusted or variable input
  - keep fuzz targets deterministic and side-effect isolated
- Module and supply-chain hygiene:
  - maintain `go.mod`/`go.sum` via Go toolchain commands; do not hand-edit casually
  - run `go mod tidy` after dependency-impacting changes
  - configure `GOPRIVATE`/`GONOSUMDB` correctly for private modules
- Security hygiene:
  - run `govulncheck ./...` on dependency changes and before releases
  - prioritize reachable vulnerabilities (call-graph relevant), not just raw CVE presence
- Version-awareness:
  - verify behavior against the targeted Go release notes when upgrading Go version
  - when adopting new toolchain/runtime features, record minimum Go version constraints explicitly
- External API enum contract (MANDATORY):
  - when official API docs define a finite set of values, model them as typed enum-like constants in Go
  - validate enum values before HTTP request execution
  - do not use ad-hoc raw string literals at call sites for documented enum fields
  - keep enum names and values synchronized with official API docs when endpoints change
- Transport client mirror rule (MANDATORY):
  - transport clients must be strict mirrors of official API documentation (endpoints, payload fields, documented enums, response/error mapping)
  - transport clients must not contain use-case/business decisions
  - use-case behavior (for example login credential verification policy) belongs to `internal/app/services/*` and adapter layer, not transport client methods

## Architecture Standard (Hexagonal / Ports And Adapters)

VERY IMPORTANT (MANDATORY):

- all new features and refactors must follow hexagonal architecture (ports and adapters)
- architecture violations are blocking issues and must be fixed before merge

Authoritative resources (required reading for contributors):

- Alistair Cockburn, original Hexagonal Architecture article:
  - https://alistair.cockburn.us/hexagonal-architecture
- AWS Prescriptive Guidance, Hexagonal architecture pattern:
  - https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/hexagonal-architecture.html
- Robert C. Martin, Clean Architecture and dependency rule context:
  - https://blog.cleancoder.com/uncle-bob/2011/11/22/Clean-Architecture.html

Reference implementation hint (optional, for practical patterns):

- some CLI structuring and hexagonal layering approaches can be taken from:
  - https://github.com/yaroslav-koval/hange
- use it for architecture/CLI ideas only; keep wbcli domain boundaries and requirements as the source of truth

Concept model:

- center (inside): business core
  - domain model + business rules (`internal/domain`)
  - application/use-case orchestration (`internal/app`)
- boundary: ports
  - inbound ports: use-case interfaces exposed by the core
  - outbound ports: interfaces the core needs for side effects (exchange API, secret store, DB, clock, etc.)
- edge (outside): adapters
  - primary/inbound adapters drive use cases (CLI, HTTP handlers, jobs)
  - secondary/outbound adapters implement outbound ports (WhiteBIT client, keychain, persistence, messaging)

Dependency rule (non-negotiable):

- dependencies point inward only
- `internal/domain` must not import adapter/infrastructure packages
- `internal/app` may depend on `domain` and port interfaces, never concrete infrastructure
- adapters depend on core ports and translate external concerns to/from core models
- root `main.go` and `cmd` package act as the composition root that wires concrete adapters into use cases

Implementation contract by layer:

- Domain (`internal/domain`)
  - pure business logic, entities/value objects, invariants, validation rules
  - no transport, IO, DB, CLI flag parsing, or framework code
- Application (`internal/app`)
  - use-cases, orchestration, transaction boundaries, policy sequencing
  - depends on domain + abstract ports only
  - defines request/response DTOs suitable for adapters
- Ports
  - declare explicit interfaces at the core boundary
  - keep interfaces small and behavior-focused (avoid god interfaces)
  - include context and error contracts where relevant
- Adapters (`internal/adapters/*`)
  - map external formats/protocols to core DTOs and back
  - isolate exchange-specific/auth-specific/storage-specific details
  - do not leak transport models into domain
- Composition root (`main.go` + `cmd`)
  - instantiate adapters
  - inject them into application services
  - select runtime configuration/profile/environment

Flow for a command/request:

1. inbound adapter parses input and performs syntax-level validation
2. adapter calls an application use-case through an inbound port
3. use-case applies domain rules and invokes outbound ports
4. outbound adapters execute side effects (API/storage/etc.)
5. use-case returns stable result model
6. inbound adapter renders output (`table`/`json`) without business logic leakage

Testing strategy tied to architecture:

- domain tests: pure unit tests, no mocks for infrastructure
- application tests: use fakes/stubs for outbound ports, validate orchestration and error mapping
- adapter tests: integration-focused contract tests against real protocol boundaries where feasible
- end-to-end smoke tests: CLI path through composition root

Mock generation and usage in tests (mandatory):

- generated mocks are stored in `mocks/`
- regenerate mocks with `make gen-mocks` (uses `configs/.mockery.yml`, requires `mockery` in PATH)
- regenerate mocks whenever interface signatures change
- prefer generated mocks for outbound dependencies in application/command tests
- set explicit expectations for expected calls and non-calls to make behavior contracts visible
- for integration-style auth tests, use mock secret-store/keychain adapters rather than real OS keychain access

Anti-patterns (prohibited):

- business rules inside CLI handlers or transport structs
- domain package importing infrastructure libraries
- direct SDK/DB calls from use-case layer without outbound port
- shared mutable globals crossing layers
- adapter-specific error/data types leaking into domain APIs

PR architecture checklist (mandatory):

- does this change keep dependencies pointing inward?
- are side effects behind outbound ports?
- is business logic placed in `domain`/`app` instead of adapters?
- can the use-case run with test doubles for infrastructure?
- are adapters thin translators rather than decision engines?

## Project Structure Contract (Mandatory)

VERY IMPORTANT (MANDATORY):

- folder responsibilities below are required for all new features and refactors
- placing code in the wrong layer/folder is a blocking issue

Recommended tree (source of truth for placement):

```text
cmd/
internal/
  domain/
  app/
    application/
    services/
    ports/
  adapters/
  cli/
mocks/
docs/
configs/
tickets/
scripts/
```

What goes where:

- `cmd/`
  - Cobra command definitions and flag parsing
  - CLI input/output wiring only
  - no business rules and no direct infrastructure calls
- `internal/domain/`
  - entities, value objects, invariants, and pure validation logic
  - no imports from adapters/transport/framework packages
- `internal/app/services/`
  - application/use-case services (orchestration and policies)
  - this is the correct location for feature services (for example auth login service)
  - may depend on `internal/domain` and `internal/app/ports` only
- `internal/app/application/`
  - application container and factory wiring for command adapters
  - exposes use-case interfaces grouped by feature areas (for example `AuthUseCases`)
  - composes concrete adapters/services in one place for runtime and test overrides
- `internal/app/ports/`
  - boundary interfaces for side effects needed by services
  - examples: `CredentialStore`, `SessionStore`, `CredentialVerifier`, `Clock`
  - keep interfaces small and behavior-focused
- `internal/adapters/`
  - concrete implementations of ports (exchange, secret store, persistence, etc.)
  - translate external protocol/data into core models and back
- `internal/cli/`
  - reusable CLI utilities shared by commands (formatters, prompt helpers, redaction helpers)
  - no domain or policy decisions
- `mocks/`
  - generated test doubles from interfaces (mockery/testify template)
  - do not hand-edit generated files; regenerate with `make gen-mocks`
- `docs/`
  - product, architecture, and operational documentation
- `configs/`
  - static config artifacts (badges, templates, build/runtime config)
- `tickets/`
  - planning and execution tracking; each non-trivial feature must have a ticket
- `scripts/`
  - local automation used by development workflow

Feature-oriented example for auth:

```text
cmd/auth/login.go
internal/domain/auth/credential.go
internal/app/application/factory.go
internal/app/services/auth/login.go
internal/app/ports/auth.go
internal/adapters/secretstore/keychain.go
```

Specific rule for current roadmap:

- implement auth services independently from order/placement client development
- keep login connectivity validation bound to WhiteBIT `POST /api/v4/collateral-account/hedge-mode`

## Documentation Update Policy

When behavior changes, update docs in the same change set:

- user-facing usage docs
- current-state sections in `AGENTS.md`
- architecture/design notes when boundaries change
- runbook/ops docs when operational steps change

If no doc impact, change notes must explicitly state: `Docs impact: none`.

## Release Cadence And Changelog

Cadence:

- default: weekly or bi-weekly release train
- hotfix releases as needed

Changelog process:

- maintain `Unreleased` section continuously
- group entries by `Added`, `Changed`, `Fixed`, `Removed`
- tag release with version and date
- link tickets and PRs in release notes

## Incident And Bug Handling

Severity:

- `SEV1`: critical outage/security
- `SEV2`: major degradation
- `SEV3`: minor issue/workaround exists

Flow:

1. open incident ticket immediately
2. stabilize service (mitigate first, optimize later)
3. capture timeline and decisions
4. ship fix with linked ticket/PR
5. complete postmortem for SEV1/SEV2 within 5 business days

## Metrics And KPIs

Delivery metrics:

- lead time (ticket start to production)
- cycle time by status
- throughput (done tickets per week)
- WIP aging

Quality metrics:

- change failure rate
- escaped defects by severity
- mean time to recovery (MTTR)
- flaky test rate
- documentation freshness (tickets merged with doc updates where required)

## Automation Expectations

CI minimum gates:

- build/lint/test required for protected branches
- secret scanning and dependency vulnerability scan
- PR template enforcement

Workflow automation:

- auto-label by path/type where possible
- stale ticket reminder at 7 days inactivity
- auto-close stale tickets at 30 days after reminder unless exempt
- scheduled reminder for weekly and monthly rituals

## Weekly And Monthly Rituals

Weekly (30-45 min):

- review in-progress items and blockers
- check aging tickets and rebalance priorities
- review escaped bugs and flaky tests
- confirm release readiness

Monthly (60 min):

- review KPI trends
- prune backlog and archive stale work
- audit test gaps and incident follow-ups
- update process rules causing repeated friction

## Templates

### Ticket Template

```md
ID: PROJ-YYYY-NNN
Title:
Priority: P0|P1|P2|P3
Status: Backlog|Ready|In Progress|Review|Blocked|Done|Archived
Owner:
Due Date: YYYY-MM-DD | None
Links: [Design Doc] [PR] [Related Tickets]

Problem:

Outcome:

Acceptance Criteria:
- [ ]
- [ ]

Risks:

Rollout Plan:

Rollback Plan:
```

### Todo Item Template

```md
- [ ] <short action> | Owner: <name> | Ticket: <ID> | Due: <YYYY-MM-DD>
```

### PR Template

```md
## Summary

## Ticket
- Primary: <ID>

## Changes
- 

## Risk
- Level: low|medium|high
- Key risks:

## Testing
- [ ] Unit
- [ ] Integration
- [ ] Manual
- Evidence:

## Docs
- [ ] Updated
- [ ] Not needed (reason): 

## Rollback

## Reviewer Checklist
- [ ] Acceptance criteria met
- [ ] Edge cases covered
- [ ] Security/privacy considered
- [ ] Observability impact considered
```

### Weekly Review Template

```md
Week Of:

Completed:
- 

In Progress:
- 

Blocked:
- 

KPIs Snapshot:
- Lead time:
- Throughput:
- Escaped defects:
- Flaky tests:

Next Week Focus:
- 
```

### Postmortem Template

```md
Incident ID:
Severity: SEV1|SEV2|SEV3
Date:
Owners:

Summary:

Customer Impact:

Timeline:
- HH:MM -

Root Cause:

Contributing Factors:

What Went Well:

What Failed:

Action Items:
- [ ] <action> | Owner: <name> | Due: <date> | Ticket: <ID>
```

## Quick Start In 30 Minutes

1. Create ticket board columns: `Backlog`, `Ready`, `In Progress`, `Review`, `Blocked`, `Done`, `Archived`.
2. Add ticket/PR/postmortem templates from this file to your platform.
3. Define branch protection for trunk mode: direct pushes to `main` allowed for the 2-developer team, CI required on `main`.
4. Enable CI checks: lint, test, build, secret scan.
5. Start triage: label current work with IDs, priorities, owners, due dates.
6. Set WIP rule (`max 2`) and stale reminder automation.
7. Schedule weekly (30-45 min) and monthly (60 min) operating reviews.

## Common Failure Modes And Prevention Rules

- Work starts without clear acceptance criteria:
  - prevention: enforce DoR before `In Progress`
- Too many parallel tasks, little completion:
  - prevention: strict WIP limit and weekly aging review
- Changes land with hidden risk:
  - prevention: mandatory risk section + rollback plan
- Docs drift from behavior:
  - prevention: docs update required in DoD
- Repeated incidents with no learning:
  - prevention: required postmortem actions tracked as tickets
- Release pain accumulates:
  - prevention: fixed cadence + changelog discipline
