# Run-error mapping (after approve)

A `202` from approve is **not** success. Poll
`GET /action/v1/action-proposals/{proposalId}` and read `data.latestAttempt`.

HTTP errors on create / approve / reject are a different envelope — see
[`http-contract.md`](http-contract.md).

## How to read `latestAttempt.error`

`error` keys are exactly: `message`, `reason`, `catalogKey`, `certainty`,
`retryable`. Example:

```json
{
  "message": "duplicatedValueError: Negative keyword already exists",
  "reason": null,
  "catalogKey": "negative_keyword.create",
  "certainty": "CERTAIN",
  "retryable": false
}
```

| Field | On | Meaning |
|---|---|---|
| `message` | `error` | What to show. Amazon 207 items: `{errorType}: {Amazon text}`. |
| `reason` | `error` | Pre-flight only (`no_items`, `lwa_token`). Amazon was **not** called. |
| `catalogKey` | `error` | `entity.action` that failed. |
| `certainty` | `error` | `CERTAIN` = known outcome. `AMBIGUOUS` / `UNKNOWN` = do **not** retry blindly. |
| `retryable` | `error` | Outcome info only. `true` does **not** mean re-approve. `null` = not recorded. |
| `providerHttpStatus` | `latestAttempt` | Amazon HTTP status (`207` = item-level, not whole-call success). |
| `providerRequestId` | `latestAttempt` | Amazon request id. Quote it when a provider call happened. |

Lookup order:

1. `error.reason` if present
2. `error.message` prefix before `:` (Amazon `errorType`)
3. exact `error.message`
4. `providerHttpStatus`
5. `proposalStatus` with `latestAttempt: null`

Tell the user the **meaning**, not the raw string alone. Quote `proposalId`,
`catalogKey`, and `providerRequestId` when a provider call happened.

`SUCCEEDED`, `FAILED`, and `UNKNOWN` are final for that proposal — **never
re-approve**. Build a **new** proposal if the user still wants the change.
`retryable: true` is not permission to re-approve.

---

## 1. Pre-flight (`reason` set, Amazon not called)

No `providerRequestId`. `providerHttpStatus` is null. `certainty` is `CERTAIN`.

| `reason` | `message` | Tell the user | Next |
|---|---|---|---|
| `no_items` | Proposal has no executable items | The proposal was empty; nothing was sent to Amazon. | Do not re-approve. Inspect GET `items`. Create a new proposal if they still want the change. |
| `lwa_token` | Amazon Ads authorization failed | Amazon Ads authorization for this account failed. The change was not sent. | Do not re-approve. The account's Ads connection needs a human fix. |

Never describe these as "Amazon errors."

---

## 2. Gateway messages (exact `message`)

| `message` | Typical HTTP | Tell the user | Next |
|---|---|---|---|
| `throttled` or `throttled; retry-after=N` | 429 | Amazon throttled the request. | Do not auto-retry. After FAILED, wait at least N seconds before a **new** proposal. |
| `unauthorized` | 401 | Amazon rejected the Ads access token. | Auth/connection problem. Do not re-approve. |
| `access_denied` | 403 | Amazon denied access for this advertising profile. | Fix profile permissions; new proposal only after that. |
| `provider_5xx` | 5xx | Amazon had a server error; the change may or may not have applied. | If status is `UNKNOWN`, verify in Ads / Data Layer first. Never resubmit the same create. |
| `item_failed` | 207/200 | The Amazon item failed without a parsed message. | Quote `providerRequestId`. Do not guess. |
| `item_error_at_index_N` | 207/200 | Amazon rejected that index; the error body was empty. | Quote `providerRequestId`. On bulk, N is the **first** failing index — not proof earlier indexes failed. |
| `provider_batch_failed` | 207/200 | The bulk call failed without a parsed message. | Quote `providerRequestId`. Do not guess. |
| network / `timeout` / `AbortError` | 0 | The call to Amazon did not complete; the change may or may not have applied. | Same as `UNKNOWN`: verify live state; do not retry blindly. |

---

## 3. Amazon `errorType` (prefix of `message`)

On HTTP 207/200, messages look like:

```text
{errorType}: {Amazon message}
```

Match the prefix case-insensitively. Prefer the text **after** `:` in the
user-facing explanation.

| Prefix / `errorType` | Tell the user | Next |
|---|---|---|
| `duplicatedValueError` | Amazon already has this entity; nothing new was created. | Stop unless they wanted a different value. Do not recreate. |
| `entityNotFoundError` | Amazon does not have that entity on this profile. | Re-query Data Layer ids. New proposal with a live id. |
| `missingValueError` | The request was missing a required Amazon field. | Rebuild parameters from catalog + a prior SUCCEEDED item. New proposal. |
| `rangeError` | Amazon rejected the number as out of range. | Ask for a value inside Amazon limits. New proposal. |
| `biddingError` | Amazon rejected the bidding change for this campaign. | Check targeting type / strategy. New proposal. |
| `budgetError` | Amazon rejected the budget. | Adjust amount/type. New proposal. |
| `parentEntityError` | The parent campaign or ad group cannot take this change. | Inspect parent state in Data Layer. |
| `invalidArgumentError` / `INVALID_ARGUMENT` / `malformedValueError` / `unsupportedValueError` / `VALIDATION_ERROR` | Amazon rejected a field value: {text after `:`}. | Fix that field. New proposal. |
| `otherError` | Amazon returned: {text after `:`}. | Do not guess beyond that text. |
| `throttlingError` | Amazon throttled this item. | Wait; new proposal if still wanted. |

Example: `duplicatedValueError: Negative keyword already exists` →
"Amazon already has this negative keyword."

---

## 4. `providerHttpStatus` when `message` is unhelpful

| Status | Next |
|---|---|
| `null` / `0` | Use `reason` or network row above |
| `200` / `207` | Use the `errorType` table |
| `401` | Auth |
| `403` | Access denied |
| `429` | Wait; new proposal later |
| other `4xx` | New proposal with a fixed body |
| `5xx` | `UNKNOWN` rules if status is UNKNOWN |

---

## 5. `proposalStatus` with no useful attempt

| Status | `latestAttempt` | Next |
|---|---|---|
| `QUEUED` / `RUNNING` | null or in-flight | Keep polling. Do not approve again. |
| `FAILED` | null | Report FAILED with no Amazon detail. Do not re-approve. |
| `FAILED` | error present | Use tables above. Do not re-approve. |
| `UNKNOWN` | may be `provider_5xx` or timeout | Verify in Data Layer / Ads. Never resubmit the same create. |
| `SUCCEEDED` | error null | Done. Every item / index succeeded. |

---

## 6. Bulk `provider_batch`

One HTTP call, N indexes. `latestAttempt.error` is the **first** failing
index (message / `errorType`), not a per-target array. `SUCCEEDED` still
requires every index to succeed.

Amazon `207` can apply some indexes and reject others in the same call. If
`proposalStatus` is `FAILED` or `UNKNOWN`:

- Do **not** re-approve this proposal.
- Do **not** assume every target is unchanged. Verify live Ads / Data Layer
  for each GET `data.items[].target_id` before building a **new** proposal for
  the ones that still need the change.
- Quote `providerRequestId`. `catalogKey` is the single shared `entity.action`.

---

## Skill output for a failed run

1. **Outcome** — `FAILED` or `UNKNOWN` (never say "Amazon error" for `lwa_token` / `no_items`)
2. **Meaning** — one sentence from the matching row
3. **Evidence** — `catalogKey`, `providerHttpStatus`, `providerRequestId`
4. **Next** — stop / verify live state / new proposal with a specific fix
5. Never re-approve a `FAILED` or `UNKNOWN` proposal
6. On bulk `FAILED`/`UNKNOWN`, say that some indexes may already have applied; verify live state before a **new** proposal
