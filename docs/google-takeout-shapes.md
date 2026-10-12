# Google Search export shapes

Research date: 2026-10-07; verified against delivered Takeout on 2026-10-11.

## Search history

Google's [My Activity schema reference](https://developers.google.com/data-portability/schema-reference/my_activity?hl=en) documents activity records in **JSON or HTML**. Search activity includes query, visit, view and advertising events. Listed fields:

| Fields | Documented meaning |
| --- | --- |
| `header`, `title` | Card label, activity summary |
| `titleUrl` | Activity URL |
| `time` | Timestamp string; example includes fractional seconds and Z |
| `products` | Product list |
| `subtitles`, `description` | Additional activity information |
| `details`, `activityControls`, `locationInfos` | Origin/settings/location context |
| `imageFile`, `audioFiles`, `attachedFiles` | Attachment filenames/references |

Fields vary by action; none should be assumed present on every record. Preserve unknown/nested fields as raw data. The reference describes Data Portability exports: it is **not a versioned Takeout JSON Schema**. Its simplified type labels do not establish concrete nested JSON structures. It does not specify a top-level JSON envelope, exact filenames, localized directory names or HTML DOM contract. Verify those against the arriving archive before implementing native parsing.

## Other Search-related exports

These are separate datasets; they must not be classified automatically as timestamped search-query history.

| Dataset | Published format and fields | Primary reference |
| --- | --- | --- |
| Search contributions | JSON; ratings/reviews/watched items, with Published/Updated, Search Query and contribution-specific fields | [Google Search contributions](https://developers.google.com/data-portability/schema-reference/search_ugc) |
| Search notifications | CSV; configuration and topic subscription identifiers/status | [Google Search notifications](https://developers.google.com/data-portability/schema-reference/search_notifications) |
| Saved collections | CSV saved items; uploaded images may accompany them | [Google Saved](https://developers.google.com/data-portability/schema-reference/save) |
| Discover | CSV follows and not-interested settings | [Google Discover](https://developers.google.com/data-portability/schema-reference/discover?hl=en) |
| Alerts | JSON subscriptions; query, language/country, delivery/filter settings | [Google Alerts](https://developers.google.com/data-portability/schema-reference/alerts) |

## Archive boundary and next verification

[Google Takeout help](https://support.google.com/accounts/answer/3024190?hl=en) documents ZIP and TGZ archive delivery, archive splitting, and an `archive_browser.html` format guide. Container format is separate from the records inside it. Inventory every delivered part before declaring coverage.

Delivered archive inventory: `takeout-20261011T151418Z-1-001.zip` contains `Takeout/My Activity/Search/MyActivity.html`; `takeout-20261011T151418Z-2-001.zip` contains YouTube history including `Takeout/YouTube and YouTube Music/history/search-history.html`, but no Google Search My Activity file. Both ZIP containers passed integrity checks.

Observed Google Search cards are HTML, one activity card per `<div class="outer-cell ...">`. Search cards use title `Search` and a first content cell containing one or more `<br>`-separated lines: action/link text, optional extra detail line, then a timestamp such as `Oct 11, 2026, 9:42:57 AM EDT`. Observed actions include `Searched for`, `Visited`, `Viewed`, `Used Search`, and `Ran internet speed test`; the speed-test shape adds a `Test run by Measurement Lab` link line before the timestamp. The raw parser keeps the card hash and locator in provenance, preserves the raw fields in metadata, infers the ZIP account from `Takeout/Google Account/*.SubscriberInfo.html`, and normalizes only `Takeout/My Activity/Search/MyActivity.html` into the `google_search` collection contract.

Current personal-search Google loader accepts the normalized archive described in [google-search.md](google-search.md), plus raw Google Takeout ZIP/HTML for the Search My Activity file. YouTube search history in part 2 is intentionally not classified as Google Search activity by this loader.
