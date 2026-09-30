Ingest report feature status

- `--report-out` selects report location; default `./ingest_report.json` in the current working directory.
- Reports remain ChatGPT-ingest-specific and record per-record classification plus stage failures.
- Acceptance coverage checks the CLI-emitted JSON report and repeat-ingest behavior.
- Integration tests send report artifacts to temporary directories.
- Removed the unused machine-specific report fixture.
- `--max_docs` limits documents ingested; report classification totals describe the full export.

Prior roadmap remains separate: keep this repo as the canonical ingest/search/MCP layer; consider semtools only as an upstream parser for messy document formats.
