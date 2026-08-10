---
description: Query the Ecombrain Data Layer (read-only Amazon ads + retail GraphQL API) to answer questions about the user's account, product/ASIN, campaign, keyword, search-term, product-target, and advertised-product performance — impressions, clicks, spend, sales, ACOS/ROAS and other KPIs — plus current FBA/merchant inventory levels, stock coverage and restock recommendations. Use whenever the user asks for Amazon advertising, sales, or inventory data, metrics, rankings, or trends from their Ecombrain account.
---

# Ecombrain Data Layer

Answer the user's Amazon performance questions by running **read-only** GraphQL
queries against the Ecombrain Data Layer with the `ecombrain-gql` command. Only
queries are allowed — never mutations or subscriptions.

**The exact schema is in [`reference.md`](reference.md) next to this file. Read it
before writing a query** so entity names, field names, and filters are correct.
Do not guess field names and do not rely on introspection.

## Queryable entities

Seven analytics entities, all daily and all sharing the same input/result shape:

| Query | Grain |
|---|---|
| `accountPerformance` | account/day (ads + retail rollup) |
| `productPerformance` | product ASIN·SKU/day |
| `campaignPerformance` | campaign/day |
| `keywordPerformance` | keyword/day |
| `searchTermPerformance` | customer search-term/day |
| `productTargetPerformance` | product-target/day |
| `advertisedProductPerformance` | product-ad/day |

Plus one snapshot entity:

| Query | Grain |
|---|---|
| `inventory` | current inventory per account·seller·ASIN·SKU (**no `dateRange`, no `groupBy`**) |

Plus metadata queries: `amazonAccounts`, `dataCatalog`, `dataAsset`,
`dataFreshness`, `_health`. (See reference.md.)

Do **not** query `brandAnalyticsProducts`, `brandAnalyticsSearch`, or
`productCosts` — they are out of scope for this skill.

## Workflow

1. **Ensure a token exists.** If unsure, run `ecombrain-config`. If no token,
   tell the user to run `/ecombrain:login` and stop until connected.

2. **Resolve accounts (usually).** Run `amazonAccounts` to get account `id`s and
   marketplaces. Pass the relevant `id`s as `accountIds`. If the user clearly
   means "everything," you may omit `accountIds` (defaults to full token scope).

3. **Pick the date range.** Every *analytics* query needs `dateRange { from, to }`
   (`YYYY-MM-DD`); `inventory` takes none — it is a snapshot. If the user is vague, ask or pick a sensible window (e.g. last
   30 days) and state which you used. Check `dataFreshness` if you need the latest
   available date.

4. **Build the query from [`reference.md`](reference.md):** choose the entity,
   select only needed fields, add `where` filters (only filterable fields work),
   `groupBy`/`orderBy` as needed, and `pagination` (`first` ≤ 500). Prefer GraphQL
   variables over string interpolation, and pass them with `--variables`.

5. **Run it:**
   ```bash
   ecombrain-gql --query '<gql>' --variables '<json>'
   ```
   or pipe the query on stdin: `echo '<gql>' | ecombrain-gql`.

6. **Paginate** when needed: request `pageInfo { hasNextPage endCursor }`, then
   pass `pagination.after = endCursor` until `hasNextPage` is false or you have
   enough. Cap the number of pages and tell the user if results were truncated.

7. **Present** results as concise tables/summaries. `Decimal` values come back as
   strings — treat them as numbers when formatting. Use `totals` for the aggregate
   line and `derived { acos roas ctr cpc cvr cpm aov }` for KPIs.

## Example — top campaigns by spend, last 30 days

```bash
ecombrain-gql \
  --query 'query($input: CampaignPerformanceInput!) {
    campaignPerformance(input: $input) {
      rows { campaignName campaignType adSpend adRevenue adOrders derived { acos roas } }
      totals { adSpend adRevenue }
      pageInfo { hasNextPage endCursor }
      queryInfo { returnedRows dateFrom dateTo }
    }
  }' \
  --variables '{
    "input": {
      "accountIds": ["<ACCOUNT_ID>"],
      "dateRange": { "from": "2026-06-23", "to": "2026-07-22" },
      "groupBy": ["campaignId"],
      "orderBy": [{ "field": "adSpend", "direction": "DESC" }],
      "pagination": { "first": 25 }
    }
  }'
```

## Example — low-stock SKUs (inventory snapshot)

```bash
ecombrain-gql \
  --query 'query($input: InventoryInput!) {
    inventory(input: $input) {
      rows {
        asin sku quantity price
        fbaInventory { fulfillableQuantity reservedQuantity totalQuantity }
        salesCoverageAndReplenishment { daysOfSupply recommendedReplenishmentQty alert }
      }
      pageInfo { hasNextPage endCursor }
      queryInfo { returnedRows }
    }
  }' \
  --variables '{
    "input": {
      "accountIds": ["<ACCOUNT_ID>"],
      "where": { "quantity": { "lte": 20 } },
      "orderBy": [{ "field": "quantity", "direction": "ASC" }],
      "pagination": { "first": 50 }
    }
  }'
```

## Example — list accounts first

```bash
ecombrain-gql --query '{ amazonAccounts { id storeName marketplaceName countryCode currencyCode isActive } }'
```

## Handling errors

- **Exit code 2 (authentication):** token missing or rejected → tell the user to
  run `/ecombrain:login`, then retry. (See the `ecombrain:authentication` skill.)
- **GraphQL errors (exit 1):** read stderr. Usually a wrong field/filter — fix it
  against `reference.md` (e.g. a field that isn't in that entity's `Where` is not
  filterable; `Decimal` filter values must be strings).
- **Unreachable API:** for local dev, confirm the GraphQL server (see
  `ecombrain-config` for the URL) is running.

## Guardrails

- Read-only only: never write `mutation`/`subscription` — `ecombrain-gql` refuses
  them anyway.
- Never print or ask the user for the raw token.
- Keep `pagination.first` ≤ 500 and select only the fields you need (queries are
  billed by bytes processed — see `queryInfo.bytesProcessed`).
