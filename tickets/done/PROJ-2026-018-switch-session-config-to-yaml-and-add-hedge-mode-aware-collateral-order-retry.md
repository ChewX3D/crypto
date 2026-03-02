# PROJ-2026-018: Switch session config to YAML and add hedge-mode-aware collateral order retry

ID: PROJ-2026-018
Title: Switch session config to YAML and add hedge-mode-aware collateral order retry
Priority: P1
Status: Done
Owner: nocle
Due Date: None
Created: 2026-03-02
Updated: 2026-03-02
Links: []

Problem:
- `~/.wbcli/config.yaml` was persisted as JSON despite `.yaml` extension.
- Order placement side mapping did not account for real hedge-mode state and could fail with `hedgeMode` mismatch.
- Hedge mode from login probe was not persisted for reuse.

Outcome:
- runtime config supports legacy JSON read + YAML read, while all writes are YAML only
- auth login persists hedge mode metadata
- collateral order placement builds request fields by hedge mode and retries once after hedge-mode mismatch by refreshing mode

Acceptance Criteria:
- [x] session config writer persists YAML to `~/.wbcli/config.yaml` and preserves `0600` file mode
- [x] session config loader reads both legacy JSON payloads and YAML payloads
- [x] login probe result persists `hedge_mode` in session metadata
- [x] order place service resolves hedge mode and maps `side`/`positionSide` accordingly
- [x] on mismatch message `hedgeMode: Order's position side does not match user's setting`, service refreshes hedge mode, saves it, and retries once
- [x] README, `docs/cli-design.md`, `docs/whitebit-integration.md`, and AGENTS current-state sections are updated

Risks:
- YAML parser is intentionally minimal and scoped to project-owned config structure.

Rollout Plan:
- merge to `main`
- users keep existing `~/.wbcli/config.yaml`; loader reads old JSON and next write migrates file to YAML automatically
- validate with `gofmt`, `go vet`, `go test`, `go build`

Rollback Plan:
- revert commit
- previous versions still read JSON format if needed

Status Notes:
- 2026-03-02: Created in Ready.
- 2026-03-02: Implemented: YAML config write + JSON/YAML read compatibility, hedge-mode persistence on login, hedge-mode-aware order mapping with mismatch refresh/retry, and docs sync.
