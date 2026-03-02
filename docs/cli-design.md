# CLI Design (MVP)

## Goal

Provide a safe, scriptable CLI for WhiteBIT collateral trading with enough structure to power a future UI.

## Product Constraints

- do not expose API secrets in command history, logs, or repo files
- all order placement supports preview/validation before execution
- default behavior is conservative (explicit confirmation for multi-order submissions)
- command output can be rendered by both humans and machines (`table` and `json`)

## Command Model

### `wbcli auth`

- `printf '%s\n%s\n' "$WBCLI_API_KEY" "$WBCLI_API_SECRET" | wbcli auth login`
- `wbcli auth status`
- `wbcli auth logout`

Implementation notes:

- store secrets using platform secret storage (Keychain/libsecret/Credential Manager)
- keep only non-sensitive metadata in `~/.wbcli/config.yaml` as YAML (`backend`, `timestamps`, `key hint`, cached `hedge_mode`)
- runtime reader must support legacy JSON payloads from older versions; writer persists YAML only
- `auth login` accepts credentials only from stdin payload (first line API key, second line API secret)
- `auth login` performs signed connectivity validation via `POST /api/v4/collateral-account/hedge-mode` before persisting credentials
- no profile concept in auth flow; single-session mode only (`logged in` or `logged out`)

### Credential Encryption and Access Plan

- backend order:
  - `os-keychain` is default and required when available
  - `encrypted-file` is allowed only as explicit fallback
- encrypted-file fallback:
  - encryption: `AES-256-GCM`
  - key derivation: `Argon2id` with random per-record salt
  - file permissions: owner-only (`0600`)
  - authenticated metadata: schema version + session metadata
- runtime access policy:
  - use stdin-only credential input for `auth login`; no credential flags
  - do not log API keys, payload, signatures, or secrets
  - support short session unlock TTL for repeated commands
  - clear plaintext buffers after signing where practical
- lifecycle policy:
  - support key rotation workflow (`auth rotate`) with cutover validation
  - support local credential revoke/delete (`auth revoke` / `auth logout`)
  - prefer restricted exchange-side API key permissions and IP allowlist where supported

### `wbcli collateral order place`

Example:

```bash
wbcli collateral order place \
  --market BTC_PERP \
  --side long \
  --amount 0.01 \
  --price 50000 \
  --client-order-id my-order-001
```

Flow:

1. validate required flags and normalize side value to one of `buy|sell|long|short` in CLI adapter
2. load credentials from single-session secure storage
3. resolve account hedge-mode from session metadata (or refresh via `/api/v4/collateral-account/hedge-mode` if missing)
4. build request shape by hedge-mode:
   - hedge mode: send `positionSide` (`long|short`) with matching `side` (`buy|sell`)
   - one-way mode: omit `positionSide`
5. sign WhiteBIT request and submit order with `postOnly=true`
6. if response contains hedge-mode mismatch (`hedgeMode: Order's position side does not match user's setting`), refresh hedge-mode, persist it, and retry once
7. print normalized output contract in `table` or `json` format

### `wbcli collateral order range`

Example:

```bash
wbcli collateral order range \
  --market BTC_PERP \
  --side buy \
  --start-price 49000 \
  --end-price 50000 \
  --step 50 \
  --amount-mode constant \
  --base-amount 0.005 \
  --dry-run
```

### Range Amount Modes

- `constant`: `amount_i = base_amount`
- `arithmetic`: `amount_i = base_amount * (start_multiplier + i * step_multiplier)`
- `geometric`: `amount_i = base_amount * ratio^i`
- `fibonacci`: `amount_i = base_amount * fib(i+1)` where multipliers are `1, 1, 2, 3, 5, ...`
- `capped-geometric`: same as geometric but hard-capped with `max_multiplier`
- `custom-list`: explicit multipliers list, for example `1,1.5,2,2.5,3`

`i` is zero-based step index.

Examples:

- arithmetic progression (`x1 x2 x3 x4 x5...`):
  - `--amount-mode arithmetic --start-multiplier 1 --step-multiplier 1`
- geometric progression (`x1 x2 x4 x8...`):
  - `--amount-mode geometric --ratio 2`
- bounded geometric (safer):
  - `--amount-mode capped-geometric --ratio 2 --max-multiplier 4`

Recommended MVP set:

1. `constant`
2. `arithmetic`
3. `geometric`
4. `capped-geometric`

`fibonacci` and `custom-list` can be Phase 2, but both are useful when tuning exposure curves.

### Range Safety Controls

- hard cap for number of generated orders
- min/max notional checks before submission
- require `--confirm` for live batch placement unless interactive confirmation succeeds
- support `--dry-run` to preview generated plan and estimated exposure

## Output Contract (UI-ready)

Commands should return a stable structure internally:

- `request_id`
- `mode` (`single` or `range`)
- `orders_planned`
- `orders_submitted`
- `orders_failed`
- `errors[]`

CLI can render this as table/json; UI can consume same object via app layer.

## MVP Delivery Order

1. key storage + auth signing
2. single order placement
3. range planning (`dry-run` only)
4. range live submission with safeguards
5. cancellation helpers (`cancel by client-order-id prefix`, `cancel batch file`)
