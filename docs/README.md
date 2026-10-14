# Docs Index

Read in this order:

1. [README.md](/Users/amoselmaliah/dev/projects/personal-search/README.md)
   Operator entrypoint. Fastest path.
2. [chatgpt_search_mvp.md](/Users/amoselmaliah/dev/projects/personal-search/docs/chatgpt_search_mvp.md)
   Actual current ChatGPT workflow.
3. [chatgpt_export_source.md](/Users/amoselmaliah/dev/projects/personal-search/docs/chatgpt_export_source.md)
   Canonical source/adapter behavior.
4. [ChatGPT.md](/Users/amoselmaliah/dev/projects/personal-search/docs/ChatGPT.md)
   Higher-level adapter + search contract.
5. [chatgpt_search_mvp_test_plan.md](/Users/amoselmaliah/dev/projects/personal-search/docs/chatgpt_search_mvp_test_plan.md)
   Coverage and acceptance bar.

6. [google-search.md](google-search.md)
   Google archive ingestion, UI/MCP source selection, compatibility checks and live verification limits.

7. [google-takeout-shapes.md](google-takeout-shapes.md)
   Primary-source export fields/formats; Takeout envelope and HTML limitations.

8. [local-runtime.md](local-runtime.md)
   Socktainer installation, Docker Compose lifecycle, and persistent database reuse.

Reference/archival:
- [PRD.md](/Users/amoselmaliah/dev/projects/personal-search/docs/PRD.md)
- [implementation.md](/Users/amoselmaliah/dev/projects/personal-search/docs/implementation.md)
- [chatgpt_conversations_json_format.md](/Users/amoselmaliah/dev/projects/personal-search/docs/chatgpt_conversations_json_format.md)

Current authority:
- operator workflow: `README.md` + `docs/chatgpt_search_mvp.md`
- source parsing behavior: `docs/chatgpt_export_source.md`
- live implementation: `cmd/`, `internal/`, tests
