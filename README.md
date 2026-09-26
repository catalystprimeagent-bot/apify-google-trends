# Google Trends Lookup

Bulk Google Trends lookup. For a list of search terms you get interest over time and related
queries (both top and rising), read directly from Trends' own JSON endpoints. No headless
browser.

That matters because the market leader times out on close to a quarter of its runs while doing
this with a full browser at 4 GB of memory. This Actor reads the same data with one HTTP request
per term plus one per requested output, at 512 MB, typically finishing in a few seconds. Every
row also reports its own fetch time and status, so you can see reliability without opening a
single result.

## Input

Give it one or more terms via `searchTerms` (a JSON array, a single string, or a comma or
newline separated list). If you're switching from another Trends tool, this Actor also accepts
whichever of these field names your existing input already uses: `terms`, `keywords`, `queries`,
`term`. Supply terms in any one of them, or mix several; they are combined and deduplicated.

| Field | Type | Default | Description |
|---|---|---|---|
| `searchTerms` | array of strings | none | Terms to look up. Supply this or any one alias below. |
| `terms` | array | none | Alias of `searchTerms`. |
| `keywords` | array | none | Alias of `searchTerms`. |
| `queries` | array | none | Alias of `searchTerms`. Accepts plain strings or `{"query": "..."}` objects. |
| `term` | string | none | Alias of `searchTerms` for a single term. |
| `timeRange` | enum | `past12Months` | One of `pastHour`, `past4Hours`, `pastDay`, `past7Days`, `past30Days`, `past90Days`, `past12Months`, `past5Years`. |
| `category` | integer | `0` | Google's numeric category ID. `0` means all categories. |
| `outputs` | array of enum | both | Which datasets to return per term: `interestOverTime`, `relatedQueries`. |
| `timeoutSeconds` | integer | `20` | Per-request timeout, clamped to 5 to 60 seconds. |

**Worldwide only.** This version does not accept a `geo` parameter, on purpose: per country
breakdowns need a wider IP footprint than a datacenter proxy reliably provides, and worldwide
results are what most buyers pulling trend data actually want. Say so plainly here rather than
leaving it to be discovered.

## Output

One dataset item per (search term x output type), so a run over 3 terms with both outputs
produces 6 rows.

```json
{
  "term": "coffee",
  "outputType": "interestOverTime",
  "timeRange": "past12Months",
  "category": 0,
  "status": "ok",
  "fetchMs": 118,
  "charged": false,
  "data": [
    { "time": "1758758400", "formattedTime": "Sep 25, 2026", "value": [42], "hasData": [true] }
  ]
}
```

`status` is `ok` (data returned), `no_data` (Google Trends had nothing for this term, time range
and category, a normal outcome, not a failure) or `error` (the request itself failed). `error`
carries a plain-language reason whenever `status` is not `ok`. A term that cannot be reached at
all still gets one row per output type, each with its own status and error, rather than being
silently dropped. The only way a run fails outright is zero usable search terms in the input.

`data`'s shape depends on `outputType`:

- `interestOverTime`: a list of `{time, formattedTime, value, hasData}` points across the
  requested time range.
- `relatedQueries`: a ranked list of related search queries as Google Trends returns them, each
  with a value, a formatted value, whether it had data, and a `ranking` field of `"top"` or
  `"rising"` — both of Google's lists are included, not just the top one.

`charged` reports whether this row's lookup was billed (see Pricing). A `no_data` row is never
charged, since it can't deliver anything to bill for.

**`relatedTopics` was removed 2026-09-26.** Google's own related-topics widget returned a
genuinely empty result, `{"default":{"rankedList":[]}}`, for every term we tested, including
large well known ones ("coffee", "nike", "bitcoin", "tesla") and worldwide and per-country
queries alike. That's Google's current response, not a bug in this Actor, but it means the
output never had anything to offer, so it isn't listed as an option any more rather than being
kept around not working. `relatedQueries` has no such gap.

## Pricing

**Free.** This Actor has no price set, so a run costs you only your own Apify platform usage.
The `charged` field in each output row stays `false` while the Actor is free.

The code does support pay-per-event billing on a single `trends-lookup` event, priced per
successfully processed row, if a price is ever set on the Apify Console. Nothing is hardcoded
in the Actor: it reads its own current price from the platform at startup and runs unmetered
when there isn't one.

## A note on reliability

Google Trends throttles by recent request volume from a source, not by a per-run cap. A single
run of a normal size (a handful of terms) is well within what this Actor has tested reliably.
If you run this Actor very frequently back to back, you may occasionally see a batch of rows
come back with `status: "error"`; that's Google's own rate limit, not a bug, and it clears on
its own after a few minutes.
