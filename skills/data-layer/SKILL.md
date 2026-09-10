---
description: Query the Commerce Spine Data Layer (read-only Amazon ads + retail GraphQL API) to answer questions about the user's account, product/ASIN, campaign, keyword, search-term, product-target, and advertised-product performance — impressions, clicks, spend, sales, ACOS/ROAS and other KPIs — plus current FBA/merchant inventory levels, stock coverage and restock recommendations, Seller Central listings, Brand Analytics search data, and product costs. Use whenever the user asks for Amazon advertising, sales, inventory, or listing data, metrics, rankings, or trends from their Commerce Spine account.
---

# Commerce Spine Data Layer

Answer the user's Amazon performance questions by running **read-only** GraphQL
queries against the Commerce Spine Data Layer with the `commercespine-gql` command. Only
queries are allowed — never mutations or subscriptions.

## The schema is discovered, not memorized

This skill does **not** ship an entity/field list. The API publishes its own
catalog, and that catalog is the source of truth — it is updated server-side, so
it is always current while any list written here would go stale.

- **Entities and fields** → ask the API: `dataCatalog`, then `dataAsset`.
- **How to build the query around them** → [`reference.md`](reference.md): the
  query envelope, filter operators, scalars, and the things the catalog does not
  report (`derived` KPIs, inventory/listing nested groups, which entities reject
  `dateRange`).

Do not guess field names, do not rely on GraphQL introspection (it is disabled
server-side), and do not assume the entity list from a previous session.

## Workflow

1. **Ensure a token exists.** If unsure, run `commercespine-config`. If no token,
   tell the user to run `/commercespine:login` and stop until connected.

2. **List the entities — always, before anything else.** This is cheap (~2 KB)
   and tells you what exists right now:

   ```bash
   commercespine-gql --query '{ dataCatalog { entity description layer queryable dateField } }'
   ```

   Read the descriptions and pick the entity whose grain matches the question.
   Ads questions are usually `L2` `*Performance` entities; stock and listing
   questions are usually `L1` snapshots.

3. **Fetch that entity's fields** (~4 KB each — only the ones you need):

   ```bash
   commercespine-gql --query '{ dataAsset(asset: "campaignPerformance") {
     entity description layer dataset table accountField dateField queryable
     fields { name type filterable groupable sortable aggregatable } } }'
   ```

   `asset` is the **entity name**, not the catalog `id`. `groupable`, `sortable`
   and `aggregatable` map straight onto `groupBy`, `orderBy` and `totals`.
   **`filterable` is not reliable** — the catalog flags almost everything
   filterable while the real `<Entity>Where` is much narrower, so take `where`
   keys from §6d of [`reference.md`](reference.md), not from the flag. See §2
   there for the full mapping, including the trap where `groupBy` silently
   returns `null` for name/label fields.

4. **Resolve accounts.** Run `amazonAccounts` to get account `id`s and
   marketplaces, and pass the relevant `id`s as `accountIds`. If the user clearly
   means "everything," omit `accountIds` (defaults to the full token scope).

   **`inventory` and `listing` require a specific `accountIds`.** They are wide
   L1 snapshots with no date range to bound them, so an unscoped query scans
   every account in the token's scope and can blow the bytes-billed cap outright.
   Ask the user which account they mean — do not fall back to the full scope for
   these two.

5. **Pick the date range.** Time-series entities **require**
   `dateRange { from, to }` (`YYYY-MM-DD`); snapshot entities (`inventory`,
   `listing`) **reject** it. §6a of [`reference.md`](reference.md) says which is
   which — note the catalog reports a `dateField` even for snapshots, so it
   cannot answer this for you. If the user is vague, pick a sensible window
   (e.g. last 30 days) and say which you used. Check `dataFreshness` when you
   need the latest available date, or when using an entity for the first time —
   `latestReportDate: null` means no data in scope.

6. **Build the query** from the catalog output plus
   [`reference.md`](reference.md): select only the fields you need, add `where`
   filters (filterable fields only), `groupBy` / `orderBy`, and `pagination`
   (`first` ≤ 500). Add `derived { acos roas ctr cpc cvr cpm aov }` for ad KPIs
   rather than computing them yourself. Prefer GraphQL variables over string
   interpolation and pass them with `--variables`.

7. **Run it:**

   ```bash
   commercespine-gql --query '<gql>' --variables '<json>'
   ```

   or pipe the query on stdin: `echo '<gql>' | commercespine-gql`.

8. **Paginate** when needed: request `pageInfo { hasNextPage endCursor }`, then
   pass `pagination.after = endCursor` until `hasNextPage` is false or you have
   enough. Cap the number of pages and tell the user if results were truncated.

9. **Present** results as concise tables/summaries. `Decimal` values come back as
   strings — treat them as numbers when formatting. Use `totals` for the
   aggregate line.

   **A missing row is not proof the thing does not exist.** Rows exist only for
   entities that were active or emitted metrics in the window you asked for, so
   an archived or paused campaign, keyword or product with no activity simply
   will not appear. Say "no data for that range" and offer a wider window —
   never "that campaign doesn't exist" or "you have no such keyword." See §6f of
   [`reference.md`](reference.md).

## Example — list what's available, then drill in

```bash
commercespine-gql --query '{ dataCatalog { entity description layer queryable } }'
```

```bash
commercespine-gql --query '{ amazonAccounts { id storeName marketplaceName countryCode currencyCode isActive } }'
```

Full worked example (catalog → fields → query) is §7 of
[`reference.md`](reference.md).

## Handling errors

- **Exit code 2 (authentication):** token missing or rejected → tell the user to
  run `/commercespine:login`, then retry. (See the `commercespine:authentication` skill.)
- **GraphQL errors (exit 1):** read stderr and match it against the error table
  in §8 of [`reference.md`](reference.md). Most are a non-filterable field in
  `where`, a `Decimal` filter passed as a number, or `dateRange` on a snapshot
  entity. Re-run `dataAsset` for the entity rather than guessing a correction.
- **Unreachable API:** for local dev, confirm the GraphQL server (see
  `commercespine-config` for the URL) is running.

## Guardrails

- Read-only only: never write `mutation`/`subscription` — `commercespine-gql` refuses
  them anyway.
- Never print or ask the user for the raw token.
- Keep `pagination.first` ≤ 500 and select only the fields you need (queries are
  billed by bytes processed — see `queryInfo.bytesProcessed`).
- Don't pull the full catalog with `fields` for every entity (~50 KB) when one
  `dataAsset` call will do.
