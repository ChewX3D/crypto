# WhiteBIT Integration Notes

Source references:

- Collateral limit order docs: https://docs.whitebit.com/api-reference/collateral-trading/collateral-limit-order
- Auth docs: https://docs.whitebit.com/private/http-auth
- Trade API index (contains bulk order endpoint): https://docs.whitebit.com/private/http-trade-v4

## Relevant Endpoints

- `POST /api/v4/collateral-account/hedge-mode` (auth connectivity probe during `wbcli auth login`)
- `POST /api/v4/order/collateral/limit`
- `POST /api/v4/order/collateral/bulk` (for batch/range placement)

## Collateral Limit Order Request Fields

Required:

- `market` (example: `BTC_PERP`)
- `side` (`buy` or `sell`)
- `amount`
- `price`

Optional:

- `clientOrderId`
- `postOnly`
- `positionSide` (`long` or `short`) when account hedge mode requires explicit position side

## Authentication and Signing

WhiteBIT private requests use headers:

- `X-TXC-APIKEY`: API key
- `X-TXC-PAYLOAD`: base64-encoded JSON payload
- `X-TXC-SIGNATURE`: hex HMAC-SHA512 of the base64 payload, signed with API secret
- `Content-Type: application/json`

Payload should include:

- `request`: endpoint path (for example `/api/v4/order/collateral/limit`)
- `nonce`: increasing numeric value
- `nonceWindow`: optional boolean for tolerant nonce validation
- request body fields (market, side, amount, etc.)

Implementation rule:

- generate payload JSON
- base64 encode payload
- sign encoded payload with HMAC-SHA512(secret)
- send payload fields as JSON body and include headers above
- for documented finite-value fields, use typed enums in code instead of raw strings (examples: `side` = `buy|sell`, `positionSide` = `long|short`)
- client must mirror official WhiteBIT documentation only (endpoints, request/response fields, documented errors), with no business/use-case logic in client methods
- business logic belongs to app services and adapters (example: login credential verification policy is adapter/service behavior, not a transport client method)

## Credential Handling Rules (CLI Side)

Derived from WhiteBIT private auth requirements (`X-TXC-APIKEY`, `X-TXC-PAYLOAD`, `X-TXC-SIGNATURE`):

- API secret is used only for request signing and must never be written to logs or command output.
- API key should be treated as sensitive operational data and redacted in diagnostics.
- Prepared payload and signature data should be kept in memory only for request lifetime.
- Nonce state should be persisted without storing secret material next to nonce cache.
- If secret backend is unavailable, fail closed by default unless encrypted fallback storage is explicitly configured.

## Nonce Strategy

Recommended for CLI:

- use millisecond timestamp as default nonce
- ensure strict monotonic increase per process
- keep a local last-nonce cache to avoid duplicate nonce on rapid calls

## API Error Handling

Normalize errors into categories:

- auth/signature/nonce errors
- validation errors (price/amount precision, invalid market)
- risk/business rule rejections
- temporary transport/server failures (retryable)

Hedge-mode recovery policy for single order placement:

- detect WhiteBIT mismatch message: `hedgeMode: Order's position side does not match user's setting`
- fetch fresh mode with `POST /api/v4/collateral-account/hedge-mode`
- persist refreshed `hedge_mode` in `~/.wbcli/config.yaml`
- rebuild request for the refreshed mode and retry once

## Range Order Mapping

For `order range`:

1. generate deterministic order plan locally
2. validate each item (precision + notional)
3. map to bulk limit order payload
4. submit in chunks if needed
5. store per-order submission results for retry and reconciliation
