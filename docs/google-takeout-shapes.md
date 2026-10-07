# Google Search export shapes

Research date: 2026-10-07. Actual incoming Takeout export is pending. Research only: no crawler executed or changed, no new raw-Takeout parser claimed.

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

When the export arrives: inspect its archive browser and member inventory; identify actual Search activity JSON/HTML; inspect a bounded sample; record field presence/types, event categories, timestamp precision/timezone, optional URL/attachments and language. Confirm account namespace explicitly. Preserve original bytes, source hash and record locator before normalization.

Current personal-search Google loader accepts only the normalized archive described in [google-search.md](google-search.md), not raw Takeout JSON/HTML/ZIP/TGZ. Existing external crawler's array layout and English Search-directory assumptions are implementation assumptions, not established by the official field reference. Native HTML parsing remains unimplemented; no timestamp timezone or DOM layout will be invented.
