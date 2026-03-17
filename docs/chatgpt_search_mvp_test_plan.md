# ChatGPT Search MVP Test Plan

## Goal

Provide an end-to-end path for Codex/ChatGPT to search a user's past ChatGPT conversations via a local MCP tool backed by this repo.

## MVP Surface

Components:
- ChatGPT export ingest CLI
- local HTTP search backend
- ChatGPT-focused search CLI
- MCP stdio wrapper exposing `search_documents`
- skill instructing Codex to use the MCP tool

## Primary User Stories

1. User ingests a ChatGPT export into a dedicated local collection.
2. User can search that collection from a local CLI.
3. Codex can search that collection through an MCP tool.
4. Search can be narrowed by `author`, `thread_id`, and date range.
5. Unsupported filters fail explicitly.
6. Skill rollout is auditable and activation can be validated.

## Test Matrix

### Unit

- request builder injects `source=["chatgpt"]`
- request builder preserves `author`, `thread_id`, and date filters
- HTTP search client handles success
- HTTP search client handles non-200 responses
- CLI validates required query
- CLI emits JSON results
- MCP handler supports:
  - `initialize`
  - `tools/list`
  - `tools/call`
- MCP tool rejects unknown tool names
- MCP tool maps arguments into the ChatGPT-scoped search request

### Integration

- ingest -> server -> CLI search returns ChatGPT results
- ingest -> server -> CLI filtered search respects `author`
- ingest -> server -> MCP search returns ChatGPT results
- ingest -> server -> MCP filtered search respects `thread_id`
- ingest -> server -> MCP unsupported `keywords` fails explicitly

### Skill Governance

- `skill_manager.py check-readiness`
- `skill_manager.py plan-install`
- `skill_manager.py apply-install --execute`
- `skill_manager.py validate-activation`

## Acceptance Bar

Complete when:
- unit and integration tests pass
- live local Qdrant e2e passes
- skill is installed through `skill-manager`
- activation is validated after install
