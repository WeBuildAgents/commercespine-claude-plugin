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

Body is the contract produced by `commercespine:action-catalog`. Send it as-is —
do not invent `executionMode` when the catalog produced a per-item body. Send
only `scope.amazonAccountId` unless the user supplied a profile/marketplace that
matches the account row.

Two shapes:

| Mode | When | Body rules |
|---|---|---|
| `per_item` (default) | One target, one or more actions | Proposal-level `target`; `actions[]` matches `items[].action` in order. `target` is `{}` when creating a new entity. |
| `provider_batch` | Same **safe-update** across **N distinct** targets | Omit proposal-level `target`; `executionMode: "provider_batch"` (or omit — API may infer); `actions` length 1; each item has its own `target`; max 1000. |

Expect **201** with `proposalId`, `proposalStatus: PENDING_APPROVAL`,
`executionMode`, `targetId` (`null` on bulk), `itemCount`, plus the server's
own `riskLevel` and `expiresAt` (pending TTL, default 24h). The response is the
proposal object, **not** wrapped in `data`.

Report `executionMode`, `targetId` / `itemCount`, and `riskLevel` — risk can be
higher than the catalog's `baseRisk`.

On **409** `TARGET_BUSY`: another PENDING/QUEUED/RUNNING proposal owns that
target (on bulk, **any** of N). Wait or use that proposal — no partial insert
happened. Do not PATCH a `provider_batch` proposal (`400`); create a new one.

Mixed catalog actions in one bulk body, bulk create/remove, and
`proposalGroupId` are refused — if create returns `400` with `details.reason`,
quote it and fix or split; do not keep a local allowlist of keys.

## 2. List

```
GET /action/v1/action-proposals                          # all, newest first
GET /action/v1/action-proposals?status=PENDING_APPROVAL  # the queue
```

`status` is the **only** supported query parameter — no `entity`, `cursor`, or
`page`. Envelope is `{ "data": [ summaries ] }`, capped at 50, without `items`.
Summaries include `executionMode` and `attemptCount`. Other status values:
`QUEUED`, `RUNNING`, `SUCCEEDED`, `FAILED`, `UNKNOWN`, `REJECTED`, `CANCELLED`,
`EXPIRED`.

## 3. Get by id (poll)

```
GET /action/v1/action-proposals/{proposalId}
```

`{ "data": { proposalId, proposalStatus, executionMode, attemptCount,
latestAttempt, items, ... } }`. Required before approve or reject, and used to
poll afterwards. There is no `/action-executions` resource — the proposal *is*
the status record.

`latestAttempt` is always present: `null` until the worker records a row. On
`FAILED` or `UNKNOWN`, read `latestAttempt.error` with
[`references/run-errors.md`](references/run-errors.md). **Never re-approve** a
terminal failure — build a **new** proposal if the user still wants the change.
`retryable: true` is outcome info only; it does not mean re-approve.

## 4. Approve — confirm with the user first

Approving enqueues a real change and is **not** undoable via cancel. Show the
user what will change (entity, action, target(s), old → new value, `riskLevel`,
and `executionMode` / `itemCount` when bulk) and get an explicit yes before
calling.

1. Resolve `proposalId`. If unknown, list `PENDING_APPROVAL` and ask which.
2. GET it. Proceed only if `proposalStatus === PENDING_APPROVAL`.
3. Then:

```
POST /action/v1/action-proposals/{proposalId}/approve
{ "reason": "<short reason, or omit>" }
```

Expect **202** `{ proposalId, proposalStatus: "QUEUED", statusUrl }`. **`202`
is not success.** Poll `statusUrl` (GET by id) until `SUCCEEDED`, `FAILED`, or
`UNKNOWN`. Re-approving something already queued returns current state and does
not start a second run. Never re-approve after `FAILED` or `UNKNOWN`.

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
- 401 / 403 `ACTION_LAYER_DISABLED` / missing capability / account not in scope;
- `proposalId` unknown and the list is empty;
- status is not `PENDING_APPROVAL` for approve or reject;
- **410** `PROPOSAL_EXPIRED` — create a new one, do not retry;
- **409** `TARGET_BUSY` (bulk: any of N) / `STALE_PRECONDITION` / `CONFLICT` —
  wait, use the in-flight proposal, or re-read current values before rebuilding;
- **400** `VALIDATION_ERROR` — quote `details.reason` (bulk shape, PATCH of
  `provider_batch`, etc.); fix or split; do not loop;
- **422** policy block, account not ready, or kill switch — do not resend the
  same body;
- **429** (including daily / concurrency limits) — wait, do not loop;
- poll ends in `FAILED` / `UNKNOWN` — map with
  [`references/run-errors.md`](references/run-errors.md); never re-approve.

Full envelopes and the HTTP error table:
[`references/http-contract.md`](references/http-contract.md).
Run failures after approve:
[`references/run-errors.md`](references/run-errors.md).

## Output

1. **Operation** and **request** (method, path, `proposalId`)
2. **Result** — HTTP status, `proposalId`, `proposalStatus`, `executionMode` /
   `itemCount` when relevant, or list count
3. **Next** — approve, poll, map `latestAttempt.error`, or stop
4. **Blockers**

On a failed or unknown run, follow the failed-run output in
[`references/run-errors.md`](references/run-errors.md) (outcome, meaning,
evidence, next). On bulk failure, say that some indexes may already have
applied and verify live state before a new proposal.

## Safeguards

- Never approve or reject without an explicit user request *and* confirmation.
- Never put tokens or secrets inside `items`.
- Never re-approve after `FAILED` or `UNKNOWN`.
- Capabilities: create `PROPOSE`, list/get `READ`, approve `APPROVE`, reject
  `REJECT`. Only `ORG_ADMIN` holds any of them today, so one token can both
  create and approve — the confirmation step is the only guard against a
  self-approved change.
