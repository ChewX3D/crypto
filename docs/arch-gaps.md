# Architectural Gap Analysis

Reference document for planned refactoring. Each finding describes what is wrong,
why it matters, and how to fix it. Open findings are at the top. Resolved findings
are at the bottom for reference.

Severity scale: **high** | **medium** | **low**

---

## Dependency Direction Violations

### GAP-004 — `SystemClock` lives in the service package instead of adapters
**Severity:** low

**What is wrong:**

`SystemClock` is a concrete type that wraps `time.Now()`. It lives inside the auth
service package:

```go
// internal/app/services/auth/clock.go
package auth

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }
```

**Why it matters:**

`SystemClock` implements the `ports.Clock` interface. All concrete implementations of
port interfaces belong in `internal/adapters/`, not in service packages. Services should
only know about the interface, not own the real implementation.

This shows up in the composition root where it has to reach into the service package
for an infrastructure type:

```go
// factory.go
clock := authservice.SystemClock{}
```

**Fix:**

Move `SystemClock` to `internal/adapters/clock/system_clock.go`. Update `factory.go`
to import from the new adapter path.

---

## WhiteBIT Transport Client Mirror Rule Violations

### GAP-005 — `PostOnly + IOC` conflict check is a business rule inside the transport client
**Severity:** medium

**What is wrong:**

The transport client validates that `PostOnly` and `IOC` cannot both be `true` at the
same time:

```go
// internal/adapters/whitebit/collateral.go — CollateralLimitOrderRequest.validate()
if request.PostOnly != nil && request.IOC != nil && *request.PostOnly && *request.IOC {
    return ErrPostOnlyIOCConflict
}
```

**Why it matters:**

The transport client should be a strict mirror of the WhiteBIT API — it sends fields
and reads responses, nothing more. Knowing that PostOnly and IOC are mutually exclusive
is a trading rule (business knowledge), not a transport shape check.

Other checks in `validate()` — like requiring non-empty `Market`, `Amount`, `Price` —
are acceptable because they prevent malformed HTTP requests. The PostOnly+IOC rule is
different: it enforces order semantics, which belongs in the service or adapter layer.

**Fix:**

Remove the `ErrPostOnlyIOCConflict` check from `CollateralLimitOrderRequest.validate()`.
Move it into `CollateralOrderExecutorAdapter.PlaceCollateralLimitOrder` or into the
collateral service.

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

Extract to a shared utility, for example `internal/ptrutil/ptrutil.go`.

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
| 1 | GAP-009 | Remove mutable global; pass factory as parameter |
| 2 | GAP-005 | Move PostOnly+IOC rule out of transport client |
| 3 | GAP-006 | Decide: wire bulk orders fully or remove dead code |
| 4 | GAP-004 | Move `SystemClock` to adapters |
| 5 | GAP-008 | Extract `boolRef` to shared utility |

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