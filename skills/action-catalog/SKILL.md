---
description: Discover what the Ecombrain Action Layer can execute by querying the live action catalog (GET /action/v1/action-catalog), map a PPC change intent to a published entity.action key, and resolve the ids the proposal needs. Use when the user asks what actions are available, what Action Layer can change on Amazon Ads, how to build a proposal body, or when turning a recommendation into a typed action. Pairs with the action-proposals skill, which submits and approves the result.
---

# Ecombrain Action Catalog

The Action Layer executes typed changes against Amazon Ads. **What it can
execute is published by the API.** This skill teaches you how to read that
publication — it deliberately carries no list of keys, entities, or parameters,
because the server changes them without shipping a new plugin.

Mapping only. Submitting is the `ecombrain:action-proposals` skill.

## Base URL and token

`/action/v1` is REST, separate from the GraphQL Data Layer, with no CLI wrapper —
use `curl`.

- **Base URL**: run `ecombrain-config` and drop the `/graphql` suffix. Use a
  different host **only when the user names it in this conversation** (e.g.
  `http://localhost:3000` for local dev). Never send the token to a host the
  user did not ask for.
- **Token**: `token` from `~/.config/ecombrain/credentials.json`, sent as
  `Authorization: Bearer <token>`. Must start `eb_live_` or `eb_dev_` — a
  Console JWT is rejected on `/action/v1`. Never print it or put it in a URL.

```bash
TOKEN=$(python3 -c "import json;print(json.load(open('$HOME/.config/ecombrain/credentials.json'))['token'])")
curl -sS -H "Authorization: Bearer $TOKEN" "$ACTION_BASE/action/v1/action-catalog"
```

## 1. List what is executable

```
GET /action/v1/action-catalog
```

Returns `{ "data": [ … ] }`. That array is the **only** executable set — a key
absent from it does not exist, whatever the user calls it.

Read the fields off the rows rather than assuming them. Each row identifies one
executable key and carries its own metadata: which object it acts on, what it
does, the ad product, the schema version to echo back, and a base risk level.
Use each row's own values; do not treat any of them as constant across rows.

Narrow to one object once you have its name **from that response**:

```
GET /action/v1/action-catalog/{entity}
```

`{entity}` must be a value the list returned — an unknown one returns **404**.
Do not guess plurals, synonyms, or Sponsored Brands/Display equivalents.

**If the catalog call fails, stop and report the HTTP status.** There is no
offline fallback here on purpose: a remembered key list is worse than no answer,
because it proposes actions the server will reject or that have changed meaning.

## 2. Map intent to a key

Using only the rows you just loaded:

1. pick one object type present in the response;
2. pick one or more actions published for it that share the same target;
3. quote the row's key back to the user.

Same target + several actions → one proposal listing several actions.
Different targets → separate proposals.

No matching row means refuse — and say which object types *are* published.
Absence from the catalog is the refusal; do not keep a separate denylist.

## 3. Resolve the target

The proposal's `target` accepts a fixed set of id fields:

`campaignId`, `adGroupId`, `keywordId`, `targetId`, `productAdId`,
`negativeKeywordId`, `negativeTargetId`

Pick the one the object type implies. A create action omits the id of the thing
being created — the provider assigns it — but still needs its parent ids.

Resolve real ids from the Data Layer before using them:
[`references/data-layer-ids.md`](references/data-layer-ids.md). Never put an id
on `target` that you have not seen in a Data Layer row.

## 4. Learn the parameters — from live data, not from memory

The catalog publishes *which* keys exist, not what goes inside `parameters`, and
the create endpoint validates `parameters` only as an opaque object — a wrong
shape is accepted and fails later at the adapter. So discover the shape:

1. **Read a previous proposal for the same key.** List proposals, find one whose
   entity and action match, and fetch it by id. Its stored items show every
   field that key actually carries, with real values.

   ```
   GET /action/v1/action-proposals?status=SUCCEEDED
   GET /action/v1/action-proposals/{proposalId}
   ```

   Note the asymmetry: you **send** `parameters` as a nested camelCase object,
   but the GET returns the stored typed columns (snake_case, with money split
   into amount and currency fields). Read the columns to learn which fields the
   key uses, then send them in request form.

2. **If no precedent exists, ask the user** for the parameter values, and say
   why: the API does not publish a schema for that key. Do not invent field
   names, and do not copy fields from a different key.

Regardless of source:

- the action you send must be one the catalog published for that entity;
- the actions list must match the items in order;
- schema version and ad product come from the catalog row you loaded;
- money is `{ "amount": "<string>", "currency": "<ISO>" }`;
- bid and budget changes should carry `preconditions.expected` holding the
  current value read from the Data Layer;
- never send Amazon SP SDK envelopes — the catalog's own parameter form is
  canonical.

Shape handed to the proposals skill:

```json
{
  "entity": "<from catalog row>",
  "actions": ["<from catalog row>"],
  "scope": { "amazonAccountId": "<id>" },
  "target": { },
  "adProduct": "<from catalog row>",
  "items": [
    {
      "action": "<from catalog row>",
      "actionSchemaVersion": "<from catalog row>",
      "parameters": { },
      "preconditions": { "expected": { } }
    }
  ]
}
```

## 5. Refuse

Stop when:

- the key is absent from the live catalog;
- the catalog call failed — report it rather than working from memory;
- you are handed a raw Amazon SP payload or an Ads console URL;
- the ask is a GraphQL mutation — the Data Layer is read-only;
- it is a bulk batch across many targets.

The server also enforces policy blocks on published keys (large budget and bid
increases are rejected with `ACTION_NOT_SUPPORTED_BY_POLICY`), and may return a
`riskLevel` **higher** than the row's base risk. Report what the server says
rather than predicting it.

## Output

1. **Catalog key** — the key plus the metadata from its row
2. **Target** — id field, the Data Layer query used, the resolved id
3. **Parameters** — the values and where the shape came from (prior proposal id,
   or the user)
4. **Blockers** — anything unresolved and what you need

Then hand off to `ecombrain:action-proposals`.

## Reference

- [`references/data-layer-ids.md`](references/data-layer-ids.md) — resolving ids
  and current values, and the traps that silently return the wrong ones
