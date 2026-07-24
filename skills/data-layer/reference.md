# Ecombrain Data Layer — schema reference

Exact fields for every queryable entity. All analytics queries are **read-only**
and share the same envelope. Read this file when building a query so field names
and filters are correct.

## Query envelope (all 7 analytics entities)

```graphql
<entity>(input: <Entity>Input!): <Entity>Result!
```

`<Entity>Input` fields:

| Field | Type | Notes |
|---|---|---|
| `accountIds` | `[String!]` | Optional. Omit to use the full token scope (all accessible Amazon accounts). |
| `dateRange` | `DateRangeInput!` | **Required** for all 7 analytics entities. `{ from: Date!, to: Date! }`, dates `YYYY-MM-DD`. |
| `where` | `<Entity>Where` | Field filters (only the fields listed per entity below are filterable). |
| `groupBy` | `[String!]` | GraphQL field names to group by (e.g. `["campaignId"]`). |
| `orderBy` | `[OrderByInput!]` | `{ field: String!, direction: ASC | DESC }`. `field` is a GraphQL field name. |
| `pagination` | `PaginationInput` | `{ first: Int, after: Cursor }`. `first` default 100, **max 500**. |

`<Entity>Result` envelope:

```graphql
{
  rows: [<Entity>Row!]!      # detail rows
  totals: <Entity>Row        # aggregated totals across the result (nullable)
  pageInfo: PageInfo!        # { hasNextPage, endCursor, hasPreviousPage, startCursor }
  queryInfo: QueryInfo!      # { requestId, returnedRows, dateFrom, dateTo, effectiveAccountIds, sourceAsset, bytesProcessed, layer, generatedAt, maximumBytesBilled }
}
```

## Filters

| Filter type | Operators | Value notes |
|---|---|---|
| `IdFilter` | `eq`, `in` | strings |
| `StringFilter` | `eq`, `ne`, `in`, `contains` | strings |
| `IntFilter` | `eq`, `ne`, `gt`, `gte`, `lt`, `lte` | integers |
| `DecimalFilter` | `eq`, `gt`, `gte`, `lt`, `lte` | **string values** (e.g. `{ gte: "100.00" }`) |
| `DateFilter` | `eq`, `gte`, `lte` | `YYYY-MM-DD` |
| `BooleanFilter` | `eq` | boolean |

## Scalars

- `Date` — ISO-8601 calendar date `YYYY-MM-DD`.
- `DateTime` — ISO-8601 UTC datetime.
- `Decimal` — arbitrary-precision decimal **serialized as a string** in responses. In `DecimalFilter`, pass values as strings too.
- `Cursor` — opaque pagination cursor (use the value from `pageInfo.endCursor`).

## DerivedMetrics (nested `derived` on every analytics row)

`acos`, `aov`, `cpc`, `cpm`, `ctr`, `cvr`, `roas` — all `Decimal`. Computed ad KPIs; request them via the `derived { ... }` sub-selection.

---

## The 7 analytics entities

For each: **Row fields** (all selectable) and **Where** (only these fields are filterable).

### 1. `accountPerformance` — daily account-level ads + retail rollup
Row: `accountId, accountName, reportDate, countryCode, currencyCode, channelName, adImpressions, adClicks, adSpend, adOrders, adConversions, adRevenue, orderedRevenue, shippedRevenue, shippedUnits, totalOrderedUnits, totalProductSales, browserSession, mobileAppSessions, derived`
Where: `accountId(Id), reportDate(Date), countryCode(String), currencyCode(String), channelName(String), adClicks(Int), adImpressions(Int), adSpend(Decimal), adRevenue(Decimal)`

### 2. `productPerformance` — daily per-product (ASIN/SKU)
Row: `accountId, reportDate, product, sku, productTitle, countryCode, currencyCode, adImpressions, adClicks, adSpend, adOrders, adConversions, adRevenue, orderedRevenue, shippedRevenue, shippedUnits, totalProductSales, averageRating, reviewCount, browserSession, mobileAppSessions, derived`
(`product` holds the ASIN.)
Where: `accountId(Id), reportDate(Date), product(String), sku(String), countryCode(String), adClicks(Int), mobileAppSessions(Int), adSpend(Decimal), adRevenue(Decimal)`

### 3. `campaignPerformance` — daily per-campaign ads
Row: `accountId, reportDate, campaignId, campaignName, campaignType, status, dailyBudget, portfolioId, portfolioName, countryCode, currencyCode, adImpressions, adClicks, adSpend, adOrders, adConversions, adRevenue, ntbOrders, ntbRevenue, derived`
Where: `accountId(Id), reportDate(Date), campaignId(Id), campaignType(String), status(String), countryCode(String), adClicks(Int), adSpend(Decimal), adRevenue(Decimal)`

### 4. `keywordPerformance` — daily per-keyword ads
Row: `accountId, reportDate, keywordId, keywordText, matchType, keywordStatus, currentBid, isBrandKeyword, adgroupId, campaignId, campaignName, campaignType, countryCode, currencyCode, adImpressions, adClicks, adSpend, adOrders, adConversions, adRevenue, derived`
Where: `accountId(Id), reportDate(Date), campaignId(Id), keywordText(String), matchType(String), isBrandKeyword(String), adClicks(Int), adSpend(Decimal), adRevenue(Decimal)`

### 5. `searchTermPerformance` — daily customer search terms
Row: `accountId, reportDate, searchTerm, target, matchType, isAsin, isBrandTerm, adgroupId, campaignId, campaignName, campaignType, countryCode, currencyCode, adImpressions, adClicks, adSpend, adOrders, adConversions, adRevenue, derived`
Where: `accountId(Id), reportDate(Date), campaignId(Id), searchTerm(String), matchType(String), isAsin(Boolean), isBrandTerm(Boolean), adSpend(Decimal), adRevenue(Decimal)`

### 6. `productTargetPerformance` — daily product-targeting ads
Row: `accountId, reportDate, targetId, target, targetStatus, currentBid, isBrandTarget, adgroupId, campaignId, campaignName, campaignType, countryCode, currencyCode, adImpressions, adClicks, adSpend, adOrders, adConversions, adRevenue, derived`
Where: `accountId(Id), reportDate(Date), campaignId(Id), target(String), targetStatus(String), isBrandTarget(Boolean), adSpend(Decimal), adRevenue(Decimal)`

### 7. `advertisedProductPerformance` — daily product-ad level
Row: `accountId, reportDate, productAdId, product, sku, productTitle, adProductStatus, adgroupId, campaignId, campaignName, campaignType, countryCode, currencyCode, adImpressions, adClicks, adSpend, adOrders, adConversions, adRevenue, adNtbOrders, adNtbRevenue, derived`
Where: `accountId(Id), reportDate(Date), campaignId(Id), product(String), sku(String), adSpend(Decimal), adRevenue(Decimal)`

---

## Metadata / catalog queries (no envelope)

### `amazonAccounts: [AmazonAccount!]!`
No arguments. Returns the accounts the token can access. Use this first to get
`id` values for `accountIds`.
`AmazonAccount`: `id!, sellerId!, marketplaceId!, marketplaceName, marketplaceRegion, storeName, accountType, countryCode, currencyCode, isActive!, connectedAdsApi!, connectedSellerCentral!`

### `dataCatalog(layer: DataLayer): [DataAsset!]!`
Lists queryable assets. `layer` is optional (`L1` | `L2` | `L3`); all current
analytics entities are `L2`.

### `dataAsset(asset: String!): DataAsset!`
Schema/metadata for one asset **by GraphQL entity name** (e.g. `"campaignPerformance"`).
`DataAsset`: `id!, entity!, description!, layer!, dataset!, table!, accountField!, dateField, queryable!, fields[DataAssetField!]!`
`DataAssetField`: `name!, type!, filterable!, groupable!, sortable!, aggregatable!`

### `dataFreshness(asset: String): [DataFreshness!]!`
Latest available `reportDate` per asset within the token scope. `asset` optional
(omit for all). `DataFreshness`: `asset!, dateField, latestReportDate, layer!`

### `_health: HealthStatus!`
`{ service!, status!, timestamp! }`. Service health check.

---

## Do NOT use

These exist in the raw schema but are intentionally out of scope for this skill —
do not build queries against them: `brandAnalyticsProducts`, `brandAnalyticsSearch`,
`productCosts`.
