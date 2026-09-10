---
description: Create, list, inspect, approve, and reject (reprove) Commerce Spine Action Layer proposals on /action/v1/action-proposals — the approval queue for Amazon Ads changes. Use when the user asks to submit or create a proposal, see all proposals or the pending approval queue, check the status of a change, approve/confirm/enqueue one, or reject/reprove one. Map the change with the action-catalog skill first.
---

# Commerce Spine Action Proposals

A proposal is a typed Amazon Ads change waiting for approval. Creating one
changes nothing; **approving one enqueues it for execution against the live
account.**

Build the body with the `commercespine:action-catalog` skill first — this skill does
not invent catalog keys.

> "reprove" / "reprovar" means **reject**, not approve.

## Base URL and token

Same as the catalog skill: base URL from `commercespine-config` with `/graphql`
stripped, or a host the user names in this conversation. Token from
`~/.config/commercespine/credentials.json`, sent as
`Authorization: Bearer <token>` — must start `cs_live_` or `cs_dev_`; a Console
JWT is rejected on `/action/v1`. Never print the token.

## Route by intent

| User asks | Operation |
|---|---|
| create / submit / propose | **create** |
| all proposals / everything | **GET** with no filter |
| queue / pending / awaiting approval | **GET** `?status=PENDING_APPROVAL` |
| approve / confirm / enqueue | **approve** |
| reject / reprove / recusar | **reject** |
| did it work / status | **GET by id** (poll) |

Never chain create → approve unless the user asked for both.

## 1. Create

```
POST /action/v1/action-proposals
Content-Type: application/json
Idempotency-Key: <fresh uuid per distinct create>
```

Body is the contract produced by `commercespine:action-catalog`. Send only
`scope.amazonAccountId` unless the user supplied a profile/marketplace that
matches the account row. `actions[]` must match `items[].action` in order.
`target` is `{}` when creating a new entity.

Expect **201** with `proposalId` and `proposalStatus: PENDING_APPROVAL`, plus
the server's own `riskLevel` and `expiresAt` (pending TTL, default 24h). The
response is the proposal object, **not** wrapped in `data`.

Report the returned `riskLevel` — it can be higher than the catalog's
`baseRisk`.

## 2. List

```
GET /action/v1/action-proposals                          # all, newest first
GET /action/v1/action-proposals?status=PENDING_APPROVAL  # the queue
```

`status` is the **only** supported query parameter — no `entity`, `cursor`, or
`page`. Envelope is `{ "data": [ summaries ] }`, capped at 50, without `items`.
Other status values: `QUEUED`, `RUNNING`, `SUCCEEDED`, `FAILED`, `UNKNOWN`,
`REJECTED`, `CANCELLED`, `EXPIRED`.

## 3. Get by id

```
GET /action/v1/action-proposals/{proposalId}
```

`{ "data": { proposalId, proposalStatus, items, ... } }`. Required before
approve or reject, and used to poll afterwards. There is no `/action-executions`
resource — the proposal *is* the status record.

## 4. Approve — confirm with the user first

Approving enqueues a real change and is **not** undoable via cancel. Show the
user what will change (entity, action, target, old → new value, `riskLevel`) and
get an explicit yes before calling.

1. Resolve `proposalId`. If unknown, list `PENDING_APPROVAL` and ask which.
2. GET it. Proceed only if `proposalStatus === PENDING_APPROVAL`.
3. Then:

```
POST /action/v1/action-proposals/{proposalId}/approve
{ "reason": "<short reason, or omit>" }
```

Expect **202** `{ proposalId, proposalStatus: "QUEUED", statusUrl }`. Poll
`statusUrl` until `SUCCEEDED` / `FAILED`. Re-approving something already queued
returns current state and does not start a second run.

## 5. Reject (reprove) — confirm with the user first

Same `PENDING_APPROVAL` gate, then:

```
POST /action/v1/action-proposals/{proposalId}/reject
{ "reason": "<short reason, or omit>" }
```

Expect `{ proposalId, proposalStatus: "REJECTED" }`. Repeat rejects are
idempotent; approving after a reject returns **409**.

Reject is not cancel. Cancel (`POST .../cancel`, author or `MANAGE_PENDING`) is
for withdrawing your own pending proposal — use it only if the user asks to
withdraw.

## Stop when

- the catalog skill refused the key, or ids are unresolved;
- 401 / 403 `ACTION_LAYER_DISABLED` / missing capability;
- `proposalId` unknown and the list is empty;
- status is not `PENDING_APPROVAL` for approve or reject;
- **410** `PROPOSAL_EXPIRED` — create a new one, do not retry;
- **409** `TARGET_BUSY` / `STALE_PRECONDITION` / `CONFLICT` — re-read current
  values before rebuilding;
- **422** policy block, account not ready, or kill switch — do not resend the
  same body;
- **429** — wait, do not loop.

Full envelopes and the error table:
[`references/http-contract.md`](references/http-contract.md).

## Output

1. **Operation** and **request** (method, path, `proposalId`)
2. **Result** — HTTP status, `proposalId`, `proposalStatus`, or list count
3. **Next** — approve, poll, or stop
4. **Blockers**

## Safeguards

- Never approve or reject without an explicit user request *and* confirmation.
- Never put tokens or secrets inside `items`.
- Capabilities: create `PROPOSE`, list/get `READ`, approve `APPROVE`, reject
  `REJECT`. Only `ORG_ADMIN` holds any of them today, so one token can both
  create and approve — the confirmation step is the only guard against a
  self-approved change.
