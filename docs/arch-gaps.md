# Architectural Gap Analysis

Reference document for planned refactoring. Each finding describes what is wrong,
why it matters, and how to fix it. Open findings are at the top. Resolved findings
are at the bottom for reference.

Severity scale: **high** | **medium** | **low**

---

## Resolved

### GAP-011 — Request envelope does not send `nonceWindow`
**Severity:** ~~medium~~ — **resolved**

The official WhiteBIT Go SDK sends `nonceWindow: true` by default in every
authenticated request. Our `privateEnvelope` was missing this field.

**Resolution:** Added `NonceWindow bool` field to `privateEnvelope` and set it
to `true` in `nextPrivateEnvelope`. Windowed mode accepts any nonce within ±5s
of server time instead of strict monotonic ordering.

---

### GAP-010 — Missing `rpi` field on `CollateralLimitOrderRequest`
**Severity:** ~~low~~ — **resolved**

The WhiteBIT API documents an `rpi` (boolean, optional) parameter on collateral
limit order endpoints. The struct was missing this field, and the ioc conflict
validation only covered `ioc+postOnly`.

**Resolution:** Added `RPI *bool` field to `CollateralLimitOrderRequest`. Renamed
`ErrPostOnlyIOCConflict` to `ErrIOCConflict` and extended validation to cover both
`ioc+postOnly` and `ioc+rpi` combinations (API error code 37).

---

### GAP-012 — Error response `code` field not included in error details
**Severity:** ~~low~~ — **resolved**

The WhiteBIT API returns structured error responses with a numeric `code` field.
Our `extractErrorMessage` parsed `message` and `errors` but ignored `code`.

**Resolution:** `extractErrorMessage` now reads the `code` field and prepends it
to the returned string (e.g. `"code 37: Validation failed: ioc: ..."`).

---

### GAP-009 — `applicationFactory` global variable is not safe for parallel tests
**Severity:** ~~low~~ — **resolved**

The application factory was stored in a package-level global variable. Tests
replaced it via `SetApplicationFactoryForTest` without any locking.

**Resolution:** Removed the global. `newRootCmd` now accepts a factory parameter.
`Execute()` passes `appcontainer.NewDefault` inline. Tests use
`NewRootCmdForTest(factory)` — no shared mutable state.

---

### GAP-008 — `boolRef` helper is copy-pasted across two service packages
**Severity:** ~~low~~ — **resolved**

The same `boolRef` function existed in `auth/login.go` and
`collateral/place_order.go`.

**Resolution:** Deleted both copies. Created generic `ptrutil.Ptr[T]` in
`internal/ptrutil/ptrutil.go`. All call sites use `ptrutil.Ptr(value)`.

---

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

---

### GAP-006 — `PlaceCollateralBulkLimitOrder` exists but nothing uses it
**Severity:** ~~medium~~ — **closed, pre-built for planned ticket**

The bulk order transport method is pre-built for PROJ-2026-008 (range live submission
via collateral bulk order endpoint). It will be wired up when that ticket moves to
implementation.
