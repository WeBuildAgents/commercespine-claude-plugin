# Commerce Spine Data Layer — query contract

**Entity and field lists are NOT in this file.** They live in the API's own
catalog and are fetched at query time, so they are never stale. This file
documents the parts the catalog cannot express: the query envelope, the filter
operators, the scalars, and the handful of things you must know that the catalog
does not report.

All queries are **read-only** — never `mutation` or `subscription`.

---

## 1. Discovery: always start with the catalog

### Step 1 — top level (always run this first)

```bash
commercespine-gql --query '{ dataCatalog { entity description layer queryable dateField } }'
```

~2 KB. Returns every queryable entity with a one-line description. Read the
descriptions and pick the entity that matches the question. Do **not** skip this
because you think you remember the entity list — entities are added and removed
server-side.

`layer` tells you the source tier: `L2` = modelled daily analytics,
`L1` = raw Seller Central report snapshots. The optional argument
`dataCatalog(layer: L1)` narrows the list.

### Step 2 — one entity's fields

```bash
commercespine-gql --query '{
  dataAsset(asset: "campaignPerformance") {
    entity description layer dataset table accountField dateField queryable
    fields { name type filterable groupable sortable aggregatable }
  }
}'
```

~4 KB per entity. **`asset` takes the GraphQL entity name** (`"campaignPerformance"`),
not the catalog `id` (`"l2.campaign_performance"` returns
`One or more filters are invalid for this query.`).

Only fetch the entities you actually need. Requesting `fields` for all entities
at once is ~50 KB — do that only if you genuinely must compare entities.

### Step 3 — check freshness when recency matters

```bash
commercespine-gql --query '{ dataFreshness { asset dateField latestReportDate layer } }'
```

Here `asset` is the catalog **`id`** (`l2.campaign_performance`), not the entity
name. `latestReportDate: null` means the asset has no data in your token scope —
check this before building a report on an entity you have not used before.

### Step 4 — resolve accounts

```bash
commercespine-gql --query '{ amazonAccounts { id storeName marketplaceName countryCode currencyCode isActive } }'
```

`AmazonAccount`: `id!, sellerId!, marketplaceId!, marketplaceName,
marketplaceRegion, storeName, accountType, countryCode, currencyCode, isActive!,
connectedAdsApi!, connectedSellerCentral!`

Pass the `id` values as `input.accountIds`. Omit `accountIds` to use the full
token scope.

### `_health`

`{ _health { service status timestamp } }` — service check, no arguments.

---

## 2. Reading the catalog into a query

Each field's four booleans map directly onto one part of the query:

| Catalog flag | Where the field may be used | If you use it anyway |
|---|---|---|
| `filterable: true` | `input.where.<field>` — **but see the warning below** | `Field "x" is not defined by type "<Entity>Where".` |
| `groupable: true` | `input.groupBy: ["<field>"]` | `One or more groupBy fields are invalid for this query.` |
| `sortable: true` | `input.orderBy: [{ field: "<field>", direction: DESC }]` | `One or more filters are invalid for this query.` (yes — orderBy reports a *filter* error) |
| `aggregatable: true` | Summed across rows when `groupBy` collapses them; appears in `totals` | See the `groupBy` null trap below |

> **⚠ `filterable` over-reports.** The catalog marks nearly every field
> `filterable: true`, but the real `<Entity>Where` input type is much narrower.
> On `campaignPerformance`, 20 fields are flagged filterable and only 9 exist in
> `CampaignPerformanceWhere` — `campaignName`, `portfolioId`, `portfolioName`,
> `dailyBudget`, `ntbOrders`, `adOrders`, `adConversions`, `adImpressions` and
> `currencyCode` are all rejected. **Use the verified lists in §6d**, not the
> catalog flag, when building `where`. `groupable`, `sortable` and
> `aggregatable` are accurate.

> **⚠ `groupBy` silently nulls everything it did not group on.** Any selected
> field that is neither listed in `groupBy` nor `aggregatable` comes back
> `null`, with no error — and this includes fields the catalog flags
> `groupable: true`. `groupBy: ["campaignId"]` nulls `campaignName`;
> `groupBy: ["keywordId","campaignId","adgroupId"]` nulls both `keywordText`
> **and** `currentBid`. So a grouped query is the wrong tool for reading labels
> or current bid/budget values. Omit `groupBy` (L2 rows are already one per
> entity per day) and narrow the date range instead, or resolve those fields in
> a second ungrouped query.

Rules that follow from the flags:

- **Dimensions** are `groupable + sortable`, never `aggregatable`.
- **Metrics** are `sortable + aggregatable`, never `groupable`.
- **Labels** (`campaignName`, `productTitle`, `portfolioName`) are `sortable` but
  neither groupable nor filterable — see both warnings above.
- A field's `type` picks its filter input type (§4).
- `fields[].name` is the **GraphQL camelCase name** — use it everywhere in the
  query. `accountField` / `dateField` / `dataset` / `table` are the underlying
  snake_case warehouse columns and are informational only; never put them in a
  query.

---

## 3. Query envelope

Every entity takes one `input` argument and returns the same four-part result.

```graphql
<entity>(input: <Entity>Input!): <Entity>Result!
```

`<Entity>Input`:

| Field | Type | Notes |
|---|---|---|
| `accountIds` | `[String!]` | Optional. Omit for the full token scope. |
| `dateRange` | `DateRangeInput!` | `{ from: Date!, to: Date! }`, `YYYY-MM-DD`. **Required on time-series entities, rejected on snapshot entities** — see §6a. |
| `where` | `<Entity>Where` | Only `filterable` fields. |
| `groupBy` | `[String!]` | Only `groupable` fields. Not accepted on snapshot entities. |
| `orderBy` | `[OrderByInput!]` | `{ field: String!, direction: ASC \| DESC }`, only `sortable` fields. |
| `pagination` | `PaginationInput` | `{ first: Int, after: Cursor }`. `first` default 100, **max 500**. |

`<Entity>Result`:

```graphql
{
  rows: [<Entity>Row!]!   # detail rows
  totals: <Entity>Row     # aggregate line across the whole result (nullable)
  pageInfo: PageInfo!     # { hasNextPage, endCursor, hasPreviousPage, startCursor }
  queryInfo: QueryInfo!   # { requestId, returnedRows, dateFrom, dateTo, effectiveAccountIds,
                          #   sourceAsset, bytesProcessed, layer, generatedAt, maximumBytesBilled }
}
```

Paginate by requesting `pageInfo { hasNextPage endCursor }` and passing
`pagination.after = endCursor` until `hasNextPage` is false. Cap your page count
and say so if you truncate.

---

## 4. Filters

The catalog's `type` determines the filter input:

| Field `type` | Filter type | Operators | Value notes |
|---|---|---|---|
| `ID` | `IdFilter` | `eq`, `in` | strings |
| `String` | `StringFilter` | `eq`, `ne`, `in`, `contains` | strings |
| `Int` | `IntFilter` | `eq`, `ne`, `gt`, `gte`, `lt`, `lte` | integers |
| `Decimal` | `DecimalFilter` | `eq`, `gt`, `gte`, `lt`, `lte` | **string values** — `{ gte: "100.00" }` |
| `Float` | `FloatFilter` | `eq`, `gt`, `gte`, `lt`, `lte` | numbers |
| `Date` | `DateFilter` | `eq`, `gte`, `lte` | `YYYY-MM-DD` |
| `DateTime` | `DateTimeFilter` | `eq`, `gte`, `lte` | ISO-8601 UTC |
| `Boolean` | `BooleanFilter` | `eq` | boolean |

A `Decimal` filter passed as a number is the single most common query error.

---

## 5. Scalars

- `Date` — ISO-8601 calendar date `YYYY-MM-DD`.
- `DateTime` — ISO-8601 UTC datetime.
- `Decimal` — arbitrary precision, **serialized as a string** in responses and
  required as a string in filters. Convert before doing arithmetic.
- `Float` — plain JSON number (Brand Analytics rates use this, not `Decimal`).
- `Cursor` — opaque; only ever pass back a `pageInfo.endCursor` value.

---

## 6. What the catalog does NOT tell you

The catalog describes **columns**. The four things below are part of the query
contract and must be read here. This section is hand-maintained — if it
contradicts a live error message, trust the error.

### 6a. Time-series vs snapshot entities

The catalog reports a `dateField` for nearly every entity, **including snapshot
entities where `dateRange` is rejected**. `dateField` is the warehouse column,
not permission to pass a date range.

| Shape | Entities | `dateRange` | `groupBy` |
|---|---|---|---|
| Time-series (`L2`) | `accountPerformance`, `productPerformance`, `campaignPerformance`, `keywordPerformance`, `searchTermPerformance`, `productTargetPerformance`, `advertisedProductPerformance`, `brandAnalyticsProducts`, `brandAnalyticsSearch` | **required** | supported |
| Snapshot (`L1`) | `inventory`, `listing` | **rejected** (`Field "dateRange" is not defined by type ...Input`) | **rejected** |
| Reference (no dates) | `productCosts` | n/a | — |

On a snapshot entity, constrain the snapshot with
`where: { reportDate: { gte: "..." } }` instead.

If you meet an entity not listed here: `layer: L1` with a `sellercentral_*_report`
table means snapshot; `layer: L2` with a `*_summary` table means time-series.
Confirm cheaply by sending the query with `pagination: { first: 1 }` — a wrong
envelope fails immediately and costs nothing.

### 6b. Derived metrics — not in the catalog

Every **time-series** row exposes a nested `derived` object that the catalog does
not list:

```graphql
derived { acos aov cpc cpm ctr cvr roas }   # all Decimal
```

Prefer these over computing KPIs yourself. Snapshot entities (`inventory`,
`listing`) have **no** `derived`.

### 6c. Nested groups — not in the catalog

`inventory` and `listing` rows carry nested objects that the catalog's `fields`
list omits entirely. They are nullable, cost nothing unless selected, and are
**not filterable**.

`InventoryRow` nested groups:

| Field | Sub-fields |
|---|---|
| `productIdentification` | `asin, productName, condition, fulfilledBy, supplier, supplierPartNo` |
| `availableAndTotalInventory` | `available, totalUnits, customerOrder, unfulfillable` (Int) |
| `fbaInventory` | `fulfillableQuantity, totalQuantity, reservedQuantity, unsellableQuantity, researchingQuantity, warehouseQuantity, futureSupplyBuyable, reservedFutureSupply` (Int) |
| `fbaInboundInventory` | `workingQuantity, shippedQuantity, receivingQuantity` (Int) |
| `inboundShipments` | `working, shipped, receiving, inbound` (Int) |
| `fulfillmentCenterInventory` | `fcProcessing, fcTransfer` (Int) |
| `merchantFulfilledInventory` | `fulfillableQuantity (Int), listingExists (Boolean)` |
| `inventoryThresholds` | `currentMonthMinimum/Maximum/VeryLow/VeryHigh`, `nextMonthMinimum/Maximum/VeryLow/VeryHigh` (Int) |
| `salesCoverageAndReplenishment` | `alert (String), daysOfSupply, daysOfSupplyAtAmazon, totalDaysOfSupply, unitsSoldLast30Days (Int), salesLast30Days (Decimal), recommendedReplenishmentQty (Int), recommendedShipDate (DateTime), maximumShipmentQuantity (Int), unitStorageSize, utilization (String)` |
| `fbaDateControls` | `reportDate, downloadDate` (Date) |
| `restockDateControls` | `reportDate, downloadDate` (Date) |

`ListingRow` has one nested group, `itemDetail` (type `ListingItemDetail`). Its
sub-fields are not enumerable from the catalog and introspection is disabled on
the server — select it only if you already know a sub-field name.

### 6d. Verified `where` fields per entity

The catalog's `filterable` flag is unreliable (§2). These lists are the actual
`<Entity>Where` inputs, hand-verified against the API. Anything not listed here
returns `Field "x" is not defined by type "<Entity>Where".`

| Entity | Filterable fields |
|---|---|
| `accountPerformance` | `accountId(Id), reportDate(Date), countryCode(String), currencyCode(String), channelName(String), adClicks(Int), adImpressions(Int), adSpend(Decimal), adRevenue(Decimal)` |
| `productPerformance` | `accountId(Id), reportDate(Date), product(String), sku(String), countryCode(String), adClicks(Int), mobileAppSessions(Int), adSpend(Decimal), adRevenue(Decimal)` |
| `campaignPerformance` | `accountId(Id), reportDate(Date), campaignId(Id), campaignType(String), status(String), countryCode(String), hasActivity(Boolean), adClicks(Int), adSpend(Decimal), adRevenue(Decimal)` |
| `keywordPerformance` | `accountId(Id), reportDate(Date), campaignId(Id), keywordText(String), matchType(String), isBrandKeyword(String), adClicks(Int), adSpend(Decimal), adRevenue(Decimal)` |
| `searchTermPerformance` | `accountId(Id), reportDate(Date), campaignId(Id), searchTerm(String), matchType(String), isAsin(Boolean), isBrandTerm(Boolean), adSpend(Decimal), adRevenue(Decimal)` |
| `productTargetPerformance` | `accountId(Id), reportDate(Date), campaignId(Id), target(String), targetStatus(String), isBrandTarget(Boolean), adSpend(Decimal), adRevenue(Decimal)` |
| `advertisedProductPerformance` | `accountId(Id), reportDate(Date), campaignId(Id), product(String), sku(String), adSpend(Decimal), adRevenue(Decimal)` |
| `inventory` | `accountId(Id), sellerId(String), asin(String), sku(String), countryCode(String), quantity(Int), reportDate(Date), downloadDate(Date)` — `price`/`businessPrice` are accepted but **broken**, see below |
| `listing` | `accountId(Id), listingId(String), sku(String), asin(String), quantity(Int), fulfillmentChannel(String), status(String), downloadDate(Date), reportDate(Date)` — `price` **broken**; `pendingQuantity`, `accountName`, `itemName` rejected |

`campaignPerformance`, `inventory` and `listing` were verified field-by-field
against the live API. The other six analytics entities are carried over from the
previously hand-maintained schema doc and were accurate for
`campaignPerformance`, but have not been re-tested. If a `where` key is
rejected, trust the error and drop it.

**Known API bugs — some filters are accepted by GraphQL but fail in the
warehouse.** The GraphQL type and the underlying column type disagree, so the
query is rejected at execution:

| Filter | Error | Workaround |
|---|---|---|
| `campaignPerformance.campaignId` (`eq` and `in`) | `No matching signature for operator = for argument types: INT64, STRING` | Filter client-side on returned rows, or use `campaignType` / `status`. |
| `inventory.price`, `inventory.businessPrice`, `listing.price` | `No matching signature for operator >= for argument types: NUMERIC, STRING` | Filter client-side. Passing a number instead fails earlier at `String cannot represent a non string value`. |

`Decimal` filters on the L2 analytics entities (`adSpend`, `adRevenue`) work
correctly — this affects `ID`-typed campaign ids and the L1 snapshot price
columns only.

**Cost warning:** `listing` with a `reportDate` filter can exceed the 5 GB
bytes-billed cap (`Query exceeded limit for bytes billed`). Always scope these
snapshots to one account first — see §6g.

### 6e. Data availability

`queryable: true` means the entity accepts a query, not that it has rows. At the
last check `brandAnalyticsProducts`, `brandAnalyticsSearch` and `listing` all
report `latestReportDate: null` in `dataFreshness`. Check `dataFreshness` before
promising a user a report from one of them.

---

### 6f. Absence of a row means "no activity", not "does not exist"

The warehouse is populated from entities that are **active** or that **emitted
performance data** in the period. Anything archived, paused, or simply idle for
the window you queried has no row to return.

So an empty result — or a campaign/keyword/product missing from a list — is
ambiguous. It can mean any of:

- the entity is archived or paused and produced no metrics in that range;
- it existed but had zero activity (no impressions, clicks or spend);
- the range predates or postdates its life;
- it genuinely does not exist.

The query cannot distinguish these. **Never report absence as non-existence.**
Say the entity returned no data for the range, and offer to widen the window or
drop a `where` filter. When the user insists something should be there, a wider
`dateRange` is the first diagnostic — if it appears with an earlier date, it was
paused or archived, not missing.

This also means counts are activity counts. "How many campaigns do I have"
answered from `campaignPerformance` is *campaigns with activity in that window*,
not the account total — say which one you are reporting.

### 6g. Scope `inventory` and `listing` to one account

These two L1 snapshots take no `dateRange`, so nothing bounds the scan but
`accountIds`. Unscoped, they read every account in the token's scope, and
`listing` in particular can exceed the 5 GB bytes-billed cap and fail outright
(`Query exceeded limit for bytes billed`).

Always pass an explicit `accountIds` for `inventory` and `listing`. If the user
has not said which account, ask — do not default to the full token scope the way
you may for the L2 analytics entities.

## 7. Worked example: catalog → query

Question: *"top campaigns by spend last 30 days, with ACOS."*

1. `dataCatalog { entity description layer }` → `campaignPerformance` is
   "Daily campaign-level advertising performance", `L2`.
2. `dataAsset(asset: "campaignPerformance") { fields { ... } }` → `campaignId`
   is `groupable`; `campaignName` is sortable but **not** groupable; `adSpend`
   is `Decimal`, `aggregatable`, `sortable`.
3. `L2` ⇒ time-series ⇒ `dateRange` required, `derived` available (§6a, §6b).
4. Check `where` keys against §6d — `adSpend` is filterable, `dailyBudget` is
   not (despite the catalog flag).
5. Decide on `groupBy`. The question wants campaign **names**, and
   `campaignName` nulls out under `groupBy` (§2) — so roll up the date range
   without `groupBy` and let `orderBy` + `first` do the work:

```bash
commercespine-gql \
  --query 'query($input: CampaignPerformanceInput!) {
    campaignPerformance(input: $input) {
      rows { campaignId campaignName campaignType adSpend adRevenue adOrders derived { acos roas } }
      totals { adSpend adRevenue }
      pageInfo { hasNextPage endCursor }
      queryInfo { returnedRows dateFrom dateTo bytesProcessed }
    }
  }' \
  --variables '{
    "input": {
      "accountIds": ["<ACCOUNT_ID>"],
      "dateRange": { "from": "2026-07-27", "to": "2026-08-26" },
      "where": { "adSpend": { "gte": "100.00" } },
      "orderBy": [{ "field": "adSpend", "direction": "DESC" }],
      "pagination": { "first": 25 }
    }
  }'
```

If you do need one row per campaign across the whole window, add
`"groupBy": ["campaignId"]` and accept `campaignName: null` — then map ids to
names with a second, ungrouped query.

Snapshot equivalent — low-stock SKUs, no `dateRange`, no `groupBy`:

```bash
commercespine-gql \
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

---

## 8. Error → fix

| Error | Cause |
|---|---|
| `Field "x" is not defined by type "<X>Where"` | `x` is not actually filterable — the catalog's `filterable` flag over-reports. Use §6d. |
| `One or more filters are invalid for this query.` | A bad `orderBy` field, a `Decimal` passed as a number, or `dataAsset` called with a catalog `id` instead of an entity name. |
| `One or more groupBy fields are invalid for this query.` | A `groupBy` field is not `groupable` (labels never are). |
| `No matching signature for operator = for argument types: INT64, STRING` | The `campaignId` filter bug (§6d). Filter client-side. |
| Rows come back with `null` labels | `groupBy` is set and the field is not groupable (§2). Not an error — drop `groupBy` or resolve names separately. |
| `Field "dateRange" is not defined by type "<X>Input"` | Snapshot entity — drop `dateRange`, use `where.reportDate` (§6a). |
| `Cannot query field "x" on type "<X>Row"` | Field name wrong or belongs to another entity. Re-run `dataAsset`; remember `derived` and nested groups are absent from the catalog. |
| `Cannot query field "x" on type "DataAsset"` | The catalog surface is exactly `id, entity, description, layer, dataset, table, accountField, dateField, queryable, fields { name type filterable groupable sortable aggregatable }` — there is nothing else to ask for. |
| Introspection error mentioning `__schema` / `__type` | Introspection is disabled server-side. Use `dataCatalog` / `dataAsset` instead. |
| Exit code 2 | Auth — see the `commercespine:authentication` skill. |
