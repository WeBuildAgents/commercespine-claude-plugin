# Resolving Action ids from the Data Layer

Proposals need real Amazon Ads ids on `target` and in parent parameters, plus
current values for `preconditions.expected`. Those come from the Data Layer.

**Discover, don't assume.** Use the `commercespine:data-layer` skill: run
`dataCatalog` to see which entities exist, then `dataAsset(asset: "<entity>")`
to see their fields. The id you need is on the entity whose grain matches the
object you are changing — a keyword id on the keyword entity, a campaign id on
the campaign entity, and so on. This file only covers what discovery will *not*
tell you.

## Naming and typing across the boundary

- The Data Layer spells the ad-group id **`adgroupId`** (all lowercase); the
  Action DTO spells it **`adGroupId`**. Map across the boundary.
- Send every Action id as a **string** — several are INT64 in the warehouse.
- `scope.amazonAccountId` is the Data Layer `accountId`. It is **not** the Ads
  `profile_id`.

## Traps that silently return wrong values

**`groupBy` nulls everything it did not group on.** Any selected field that is
neither in `groupBy` nor aggregatable comes back `null`, with no error —
including fields the catalog flags `groupable`. This bites exactly the fields
proposals need: grouping by keyword id nulls both the keyword text and the
current bid. Read ids and current values from an **ungrouped** query over a
single recent day; the L2 rows are already one per entity per day.

**Two different things are called `targetId`.** The product-target id lives on
the product-target entity. The search-term entity also exposes a `targetId`,
which is a different concept — never use it as a product-target id.

**Search terms are not keywords.** The search-term entity carries no keyword id.
Use its campaign and ad-group ids as parents when creating a keyword, and treat
the search term itself as text.

**You cannot filter by `campaignId`.** `where: { campaignId: { eq: "…" } }`
fails with `No matching signature for operator = for argument types: INT64,
STRING`, and `in` fails the same way. Fetch rows and filter client-side, or
narrow with other filterable fields first. See §6d of the `commercespine:data-layer`
reference.

## What the Data Layer does not have

Discovery will show you these are missing; the point is not to stall when they
are:

- **negative keyword and negative product-target ids** — required to remove a
  negative, and not exposed anywhere. Ask the user.
- **ad-group default bid** — the current value for a default-bid precondition is
  not on any entity. Ask the user.

## After a proposal succeeds

GraphQL `recentActionChanges` (last 48h) returns each item's target id,
including the provider-assigned id after a create — that is how you learn the
id of something the Action Layer just made. It is not a substitute for reading
current budget or bid before the next change.
