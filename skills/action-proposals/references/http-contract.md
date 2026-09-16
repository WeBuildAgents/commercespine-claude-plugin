# Action proposals HTTP contract

Envelopes for `/action/v1/action-proposals`. Prefer the live response over this
file whenever a call succeeds — this is here for shapes you need *before* the
first call, and for error triage.

Auth: `Authorization: Bearer <cs_live_… | cs_dev_…>`.
Capabilities: create `PROPOSE`, list/get `READ`, approve `APPROVE`, reject
`REJECT`.

## Create — `POST /action/v1/action-proposals` → 201

Body comes from the `commercespine:action-catalog` skill. `scope` carries
`amazonAccountId`; `advertisingProfileId` and `marketplaceId` are optional and
must match the account row when sent.

Every value marked `<from catalog>` is copied off the catalog row you loaded —
never hardcoded here.

### Per-item (one target, one or more actions)

```json
{
  "entity": "<from catalog>",
  "actions": ["<from catalog>"],
  "scope": { "amazonAccountId": "<accountId>" },
  "target": { "<idField>": "<id from the Data Layer>" },
  "adProduct": "<from catalog>",
  "items": [
    {
      "action": "<from catalog>",
      "actionSchemaVersion": "<from catalog>",
      "parameters": { },
      "preconditions": { "expected": { } }
    }
  ]
}
```

### Bulk `provider_batch` (same safe-update, N distinct targets)

Omit proposal-level `target`. Set `executionMode: "provider_batch"` (or omit it
— the API infers bulk when there is no proposal `target` and every item has a
distinct non-empty target id). `actions` length is 1; each item carries its own
`target`; max 1000 items. Create/remove and mixed actions are rejected with
`400 VALIDATION_ERROR` (`details.reason`).

```json
{
  "entity": "keyword",
  "actions": ["update_status"],
  "executionMode": "provider_batch",
  "scope": { "amazonAccountId": "<accountId>" },
  "adProduct": "SPONSORED_PRODUCTS",
  "items": [
    {
      "action": "update_status",
      "actionSchemaVersion": "1.0",
      "parameters": { "state": "PAUSED" },
      "target": { "keywordId": "111" },
      "preconditions": { "expected": { "state": "ENABLED" } }
    },
    {
      "action": "update_status",
      "actionSchemaVersion": "1.0",
      "parameters": { "state": "PAUSED" },
      "target": { "keywordId": "222" },
      "preconditions": { "expected": { "state": "ENABLED" } }
    }
  ]
}
```

Response is the proposal object, **not** wrapped in `data`:

```json
{
  "proposalId": "prop_123",
  "proposalStatus": "PENDING_APPROVAL",
  "entity": "<echoed back>",
  "actions": ["<echoed back>"],
  "executionMode": "per_item",
  "scope": {
    "amazonAccountId": "amz_1",
    "advertisingProfileId": "1234567890",
    "marketplaceId": "ATVPDKIKX0DER"
  },
  "targetId": "987654321",
  "itemCount": 2,
  "riskLevel": "HIGH",
  "validFromAt": "2026-08-27T15:00:00Z",
  "expiresAt": "2026-08-28T15:00:00Z"
}
```

On `provider_batch`, `targetId` is `null` and `itemCount` is N.
`executionMode` is always present on create, list, and GET.

`expiresAt` applies to `PENDING_APPROVAL` only (default 24h).

`items[].parameters` is validated only as an object — the API will accept a
wrong parameter shape and fail later at the adapter. Learn a key's parameter
fields from a previous proposal for that key (see below), not from memory.

Do not PATCH a `provider_batch` proposal — the API returns `400 VALIDATION_ERROR`
with `details.reason=provider_batch proposals cannot be patched in V1`. Create a
new proposal instead.

## Reading a stored item — `GET /action/v1/action-proposals/{proposalId}`

The GET returns items as stored **typed columns**, not in the shape you posted:
snake_case names (`target_id` on each item row), money split into separate
amount and currency columns, unused columns present as `null`, and free-form
extras in `*_jsonb` fields. Use it to learn which fields a given action
actually populates, then translate back into the nested camelCase `parameters`
form when you create.

## List — `GET /action/v1/action-proposals[?status=…]` → 200

Optional `status` is the only query key. Newest first, capped at 50, summaries
only (no `items`). Empty → `{ "data": [] }`.

The sample below is a captured response — read it for field names and shape. The
`entity` / `actions` values in it are whatever that org happened to propose, not
a catalog.

```json
{
  "data": [
    {
      "proposalId": "prop_63278be3d8e44b7eb7f8ba1c",
      "proposalStatus": "FAILED",
      "entity": "campaign",
      "actions": ["create"],
      "executionMode": "per_item",
      "amazonAccountId": "amz_console_account_1",
      "riskLevel": "HIGH",
      "validFromAt": "2026-08-19T17:04:50.701Z",
      "queuedAt": "2026-08-19T17:07:42.468Z",
      "startedAt": "2026-08-20T16:25:11.712Z",
      "finishedAt": "2026-08-20T16:26:17.136Z",
      "attemptCount": 2,
      "createdAt": "2026-08-19T17:04:50.918Z",
      "updatedAt": "2026-08-20T16:26:17.136Z"
    }
  ]
}
```

`attemptCount > 1` means the worker retried — worth mentioning when reporting a
`FAILED` proposal.

## Get / poll — `GET /action/v1/action-proposals/{proposalId}` → 200

`{ "data": { … } }` always includes `executionMode`, `attemptCount`, and
`latestAttempt` (`null` until the worker records an attempt):

```json
{
  "data": {
    "proposalId": "prop_123",
    "proposalStatus": "FAILED",
    "entity": "campaign",
    "actions": ["update_budget"],
    "executionMode": "per_item",
    "amazonAccountId": "amz_1",
    "riskLevel": "MEDIUM",
    "attemptCount": 1,
    "latestAttempt": {
      "attemptNumber": 1,
      "startedAt": "2026-09-09T00:00:00.000Z",
      "finishedAt": "2026-09-09T00:00:01.000Z",
      "durationMs": 1000,
      "providerRequestId": "rid-9",
      "providerHttpStatus": 429,
      "confirmationSource": null,
      "providerAcceptedAt": null,
      "outcomeCertainty": "CERTAIN",
      "error": {
        "message": "throttled; retry-after=30",
        "reason": null,
        "catalogKey": null,
        "certainty": "CERTAIN",
        "retryable": true
      }
    },
    "items": []
  }
}
```

Status after approve: `QUEUED` → `RUNNING` → `SUCCEEDED` | `FAILED` | `UNKNOWN`.

`error` keys are exactly `message`, `reason`, `catalogKey`, `certainty`,
`retryable`. How to interpret them:
[`run-errors.md`](run-errors.md).

## Approve — `POST .../{proposalId}/approve` → 202

Body `{ "reason": "<optional>" }`.

```json
{ "proposalId": "prop_123", "proposalStatus": "QUEUED", "statusUrl": "/action/v1/action-proposals/prop_123" }
```

`202` is not success — poll `statusUrl` (GET by id) until a terminal status.

## Reject — `POST .../{proposalId}/reject` → 200/201

Body `{ "reason": "<optional>" }`.

```json
{ "proposalId": "prop_123", "proposalStatus": "REJECTED" }
```

## Errors

```json
{ "code": "<ErrorCode>", "message": "<human message>", "details": {}, "requestId": "req_123" }
```

| HTTP | Code | Meaning / next |
|---|---|---|
| 400 | `VALIDATION_ERROR` | Shape, unpublished key, bulk rules, or PATCH of `provider_batch`. Read `details.reason`; fix the body; do not loop. |
| 401 | `UNAUTHENTICATED` | no token, or a Console JWT on `/action/v1` |
| 403 | `ACTION_LAYER_DISABLED` / `FORBIDDEN_SCOPE` / `FORBIDDEN_ACCOUNT_SCOPE` | org plan gate, missing capability, or account not on the token — stop |
| 404 | `NOT_FOUND` | unknown `proposalId` in this org |
| 409 | `TARGET_BUSY` | another PENDING/QUEUED/RUNNING proposal on the same target; on bulk, **any** of N. Wait or use that proposal — no partial insert |
| 409 | `STALE_PRECONDITION` / `CONFLICT` | expected value moved, or wrong status — re-read current values / GET first |
| 410 | `PROPOSAL_EXPIRED` | past `expiresAt` — create a new proposal; do not approve |
| 422 | `ACTION_ACCOUNT_NOT_READY` / `ACTION_SCOPE_MISMATCH` / `ACTION_NOT_SUPPORTED_BY_POLICY` / `ACTION_KILL_SWITCH_ACTIVE` | do not resend the same body |
| 429 | `RATE_LIMIT_EXCEEDED` / `DAILY_LIMIT_EXCEEDED` / `CONCURRENCY_LIMIT_EXCEEDED` | org quotas — wait; do not loop |

Quote `requestId` when reporting a failure — it is the server-side trace handle.

Run failures after approve (`latestAttempt.error`) are not this envelope — see
[`run-errors.md`](run-errors.md).
