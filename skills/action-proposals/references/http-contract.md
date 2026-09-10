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

Response is the proposal object, **not** wrapped in `data`:

```json
{
  "proposalId": "prop_123",
  "proposalStatus": "PENDING_APPROVAL",
  "entity": "<echoed back>",
  "actions": ["<echoed back>"],
  "scope": {
    "amazonAccountId": "amz_1",
    "advertisingProfileId": "1234567890",
    "marketplaceId": "ATVPDKIKX0DER"
  },
  "targetId": null,
  "riskLevel": "HIGH",
  "validFromAt": "2026-08-27T15:00:00Z",
  "expiresAt": "2026-08-28T15:00:00Z"
}
```

`expiresAt` applies to `PENDING_APPROVAL` only (default 24h).

`items[].parameters` is validated only as an object — the API will accept a
wrong parameter shape and fail later at the adapter. Learn a key's parameter
fields from a previous proposal for that key (see below), not from memory.

## Reading a stored item — `GET /action/v1/action-proposals/{proposalId}`

The GET returns items as stored **typed columns**, not in the shape you posted:
snake_case names, money split into separate amount and currency columns, unused
columns present as `null`, and free-form extras in `*_jsonb` fields. Use it to
learn which fields a given action actually populates, then translate back into
the nested camelCase `parameters` form when you create.

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

## Get — `GET /action/v1/action-proposals/{proposalId}` → 200

```json
{ "data": { "proposalId": "prop_123", "proposalStatus": "PENDING_APPROVAL", "entity": "campaign", "actions": ["create"], "items": [] } }
```

Status after approve: `QUEUED` → `RUNNING` → `SUCCEEDED` | `FAILED`, or
`UNKNOWN`.

## Approve — `POST .../{proposalId}/approve` → 202

Body `{ "reason": "<optional>" }`.

```json
{ "proposalId": "prop_123", "proposalStatus": "QUEUED", "statusUrl": "/action/v1/action-proposals/prop_123" }
```

## Reject — `POST .../{proposalId}/reject` → 200/201

Body `{ "reason": "<optional>" }`.

```json
{ "proposalId": "prop_123", "proposalStatus": "REJECTED" }
```

## Errors

```json
{ "code": "<ErrorCode>", "message": "<human message>", "details": {}, "requestId": "req_123" }
```

| HTTP | Code | Meaning |
|---|---|---|
| 401 | `UNAUTHENTICATED` | no token, or a Console JWT on `/action/v1` |
| 403 | `ACTION_LAYER_DISABLED` / `FORBIDDEN_SCOPE` | org plan gate, or the token's role lacks the capability |
| 404 | `NOT_FOUND` | unknown `proposalId` in this org |
| 409 | `TARGET_BUSY` / `STALE_PRECONDITION` / `CONFLICT` | another change in flight, expected value moved, or wrong status |
| 410 | `PROPOSAL_EXPIRED` | past `expiresAt` — create a new proposal |
| 422 | `ACTION_ACCOUNT_NOT_READY` / `ACTION_SCOPE_MISMATCH` / `ACTION_NOT_SUPPORTED_BY_POLICY` / `ACTION_KILL_SWITCH_ACTIVE` | do not resend the same body |
| 429 | `RATE_LIMIT_EXCEEDED` | wait |

Quote `requestId` when reporting a failure — it is the server-side trace handle.
