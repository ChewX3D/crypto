# Architectural Gap Analysis

Reference document for planned refactoring. Each finding describes what is wrong,
why it matters, and how to fix it. Open findings are at the top. Resolved findings
are at the bottom for reference.

Severity scale: **high** | **medium** | **low**

---

## WhiteBIT Transport Client Mirror Rule Violations

### GAP-010 — Missing `rpi` field on `CollateralLimitOrderRequest`
**Severity:** low

**What is wrong:**

The WhiteBIT API documents an `rpi` (boolean, optional) parameter on both the
collateral limit order and bulk limit order endpoints. Our `CollateralLimitOrderRequest`
struct does not have this field.

The `ErrPostOnlyIOCConflict` validation is also incomplete — API error code 37 covers
`ioc=true + postOnly=true` AND `ioc=true + rpi=true`, but without the `rpi` field we
can't validate the second combination.

**Fix:**

Add `RPI *bool json:"rpi,omitempty"` to `CollateralLimitOrderRequest`. Add a client-side
validation for `ioc+rpi` alongside the existing `ioc+postOnly` check.

---

### GAP-011 — Request envelope does not send `nonceWindow`
**Severity:** medium

**What is wrong:**

The official WhiteBIT Go SDK (`whitebit-exchange/go-sdk/options.go`) sends
`nonceWindow: true` by default in every authenticated request:

```go
// Official SDK — options.go
type AuthParams struct {
    Request     string `json:"request"`
    Nonce       int64  `json:"nonce"`
    NonceWindow bool   `json:"nonceWindow"`
}

func NewAuthParams(url string) AuthParams {
    return AuthParams{Nonce: time.Now().UnixMilli(), NonceWindow: true, Request: url}
}
```

Our `privateEnvelope` only sends `request` and `nonce` — no `nonceWindow` field.

**Why it matters:**

Without `nonceWindow: true`, the API uses strict monotonic nonce ordering — each request
must have a nonce higher than the previous one. With `nonceWindow: true`, the API
accepts any nonce within ±5 seconds of server time and only checks uniqueness. The
windowed mode is more resilient to clock drift and concurrent requests.

**Fix:**

Add `NonceWindow bool json:"nonceWindow"` to `privateEnvelope` and set it to `true`
in `nextPrivateEnvelope`.

---

### GAP-012 — Error response `code` field not included in error details
**Severity:** low

**What is wrong:**

The WhiteBIT API returns structured error responses with a numeric `code` field that
has specific meaning per endpoint:

```json
{"code": 37, "message": "Validation failed", "errors": {"ioc": ["..."]}}
```

Documented codes for the limit order endpoint: `30` (default), `31` (market), `32`
(amount), `33` (price), `36` (clientOrderId), `37` (ioc+postOnly/rpi).

Our `extractErrorMessage` in `client.go` parses `message` and `errors` but ignores the
`code` field. The numeric code never reaches the user or the error detail string.

**Why it matters:**

The `code` gives precise context about what went wrong. The `message` field is often
generic ("Validation failed") while `code` tells you exactly which validation rule
fired. Including it in the error detail string makes errors more actionable.

**Fix:**

In `extractErrorMessage`, read the `code` field from the JSON response and prepend it
to the returned message string (e.g. `"code 37: Validation failed: ioc: ..."`). No new
struct field needed — just include it in the text that already flows into `APIError.Details`.

---

### GAP-006 — `PlaceCollateralBulkLimitOrder` exists but nothing uses it
**Severity:** medium

**What is wrong:**

The transport client has a `PlaceCollateralBulkLimitOrder` method with tests, but:

- no port interface method exists for bulk orders
- no adapter method wraps it
- no service or command calls it

**Why it matters:**

The mirror rule says the client should have "nothing more and nothing less" than what
the product needs from the API. An unused transport method adds maintenance cost for
a feature that does not exist yet. Every time the client changes, this method and its
tests have to be kept in sync even though nothing uses them.

**Fix:**

Either wire it up fully (port method, adapter method, service, command) or remove it
until the feature is planned. Do not leave half-built vertical slices at the transport
layer.

---

## DRY Violations

### GAP-008 — `boolRef` helper is copy-pasted across two service packages
**Severity:** low

**What is wrong:**

The same helper function exists in two places:

```go
// internal/app/services/auth/login.go
// internal/app/services/collateral/place_order.go — identical copy
func boolRef(value bool) *bool {
    allocated := value
    return &allocated
}
```

**Why it matters:**

Duplicated code means two places to update if the pattern changes, and two places
where bugs can hide independently.

**Fix:**

Delete both `boolRef` copies. Create a generic `Ptr[T]` helper in
`internal/ptrutil/ptrutil.go` that works for any type:

```go
func Ptr[T any](v T) *T { return &v }
```

Replace `boolRef(true)` calls with `ptrutil.Ptr(true)`. The generic version also
covers future `*string`, `*int`, etc. pointer needs without new helpers.

---

## Concurrency Safety

### GAP-009 — `applicationFactory` global variable is not safe for parallel tests
**Severity:** low

**What is wrong:**

The application factory is stored in a package-level global variable:

```go
// cmd/application_runtime.go
var applicationFactory = appcontainer.NewDefault
```

`SetApplicationFactoryForTest` replaces this global without any locking.

**Why it matters:**

This works today because tests run sequentially. But if tests ever run in parallel
(for example with `go test -race ./...`), two tests replacing the same global at the
same time is a data race. The race detector will flag it.

**Fix:**

Remove the global. Pass the factory as a parameter into `newRootCmd`.
`NewRootCmdForTest()` already accepts a factory at construction — make the production
path work the same way. The `NewDefault` wiring lives in `main.go` and gets passed in,
never stored as a mutable global.

---

## Implementation Order

| Order | Finding | Reason |
|-------|---------|--------|
| 1 | GAP-011 | Add `nonceWindow: true` to match official SDK behavior |
| 2 | GAP-010 | Add missing `rpi` field and complete ioc+rpi validation |
| 3 | GAP-012 | Include error `code` in detail string for actionable errors |
| 4 | GAP-009 | Remove mutable global; pass factory as parameter |
| 5 | GAP-006 | Decide: wire bulk orders fully or remove dead code |
| 6 | GAP-008 | Extract `boolRef` to shared utility |

---

## Resolved

### GAP-001 — `internal/app/application/factory.go` imports concrete adapters
**Severity:** ~~high~~ — **closed, accepted as-is**

`internal/app/application/` is the composition root of this project. The composition
root must import all concrete adapter packages — that is its job. `NewDefault()` wires
`configstore`, `secretstore`, and `whitebit` adapters into services; this is correct
and intentional.

The remaining constructors (`New`, `NewWithUseCases`, `NewWithAuthServices`,
`NewWithServices`) accept interfaces only and are fully clean.

**Decision:** Keep `NewDefault()` in `factory.go`. `internal/app/application/` is the
one package inside `internal/` that is explicitly permitted to import concrete adapters,
because it IS the composition root.

---

### GAP-002 — `cmd/order/errors.go` imported WhiteBIT adapter directly
**Severity:** ~~high~~ — **resolved** (commit `c8f0b0f`)

`cmd/order/errors.go` imported the WhiteBIT adapter package to check error types from
a failed order. The command layer was reading WhiteBIT-specific error values — if the
adapter is ever replaced, the command code breaks for no reason.

**Resolution:** Introduced unified `ports.APIError` type. The WhiteBIT adapter now
converts transport errors into `*ports.APIError` at the boundary. The command layer
checks `errors.As(err, &apiErr)` — no adapter import needed.

---

### GAP-007 — Error classification helpers were copy-pasted across two packages
**Severity:** ~~high~~ — **resolved** as part of GAP-002 fix (commit `c8f0b0f`)

The same string-matching logic (`indicatesMissingEndpointAccess`, `extractErrorDetail`)
existed in both `cmd/order/errors.go` and `internal/adapters/whitebit/credential_verifier.go`.
The duplication happened because `cmd/order` bypassed the port boundary and had to
re-implement classification that the adapter already did.

**Resolution:** Fixing GAP-002 eliminated this. The helpers now live only in
`internal/adapters/whitebit/apierror.go`.

---

### GAP-003 — `cmd/auth/errors.go` imported `ErrNotLoggedIn` from service package
**Severity:** ~~low~~ — **resolved**

`cmd/auth/errors.go` imported the auth service package just to use `ErrNotLoggedIn`.
This error meant the same thing as `ports.ErrCredentialNotFound` — both produced the
identical user message: "not logged in; run wbcli auth login first".

**Resolution:** Deleted `ErrNotLoggedIn` entirely. `ports.ErrCredentialNotFound` is
the single error for "not logged in". Removed the `authservice` import from
`cmd/auth/errors.go` and deleted `internal/app/services/auth/errors.go`.

---

### GAP-004 — `SystemClock` lived in the service package instead of adapters
**Severity:** ~~low~~ — **resolved**

`SystemClock` was a concrete `ports.Clock` implementation living in the auth service
package. The composition root had to reach into a service package for an infrastructure
type (`authservice.SystemClock{}`).

**Resolution:** Renamed to `clock.Real` and moved to `internal/adapters/clock/real.go`.
The composition root now imports from adapters like every other concrete dependency.
Deleted `internal/app/services/auth/clock.go`.

---

### GAP-005 — `PostOnly + IOC` conflict check in the transport client
**Severity:** ~~medium~~ — **closed, not a violation**

The WhiteBIT API documents error code `37` specifically for the `postOnly=true` +
`ioc=true` combination. The client-side check mirrors a documented API constraint —
same category as validating enum values or required fields. It prevents a wasted HTTP
call for a request the API will definitely reject.

**Decision:** Keep the check in `CollateralLimitOrderRequest.validate()`. This is
transport-level input validation, not a business rule.