SHELL := /bin/bash

QDRANT_URL ?= http://localhost:6333
DOCKER_CONTEXT ?= socktainer
QDRANT_IMAGE ?= qdrant/qdrant:v1.19.2
QDRANT_VOLUME_NAME ?= personal-search-qdrant-data
export QDRANT_IMAGE QDRANT_VOLUME_NAME
COMPOSE = docker --context "$(DOCKER_CONTEXT)" compose -f docker-compose.yml
CHATGPT_COLLECTION ?= chatgpt_messages
CHATGPT_SERVER_ADDR ?= 127.0.0.1:18080
CHATGPT_SERVER_URL ?= http://$(CHATGPT_SERVER_ADDR)
CHATGPT_EMBEDDER ?= ollama
OLLAMA_URL ?= http://localhost:11434
OLLAMA_MODEL ?= nomic-embed-text:latest
CHATGPT_DIM ?= 0
CHATGPT_BATCH ?= 8
CHATGPT_FORMAT ?= json
CHATGPT_SEARCH_ARGS ?=
REPORT_OUT ?= ingest_report.json

.PHONY: qdrant-up qdrant-down qdrant-status default-status chatgpt-doctor chatgpt-ingest chatgpt-server chatgpt-search mcp chatgpt-mcp chatgpt-snapshot chatgpt-status smoke acceptance integration semantic-smoke

qdrant-up:
	@set -euo pipefail; \
	if curl --connect-timeout 1 --max-time 2 -sf "$(QDRANT_URL)/collections" >/dev/null 2>&1; then \
		echo "qdrant already reachable at $(QDRANT_URL)"; \
	else \
		if ! command -v docker >/dev/null 2>&1; then \
			echo "Docker CLI missing. See docs/local-runtime.md for Socktainer installation." >&2; \
			exit 1; \
		fi; \
		if ! docker --context "$(DOCKER_CONTEXT)" info >/dev/null; then \
			echo "Docker context '$(DOCKER_CONTEXT)' unavailable. Start Socktainer; see docs/local-runtime.md." >&2; \
			exit 1; \
		fi; \
		if ! docker --context "$(DOCKER_CONTEXT)" volume inspect "$(QDRANT_VOLUME_NAME)" >/dev/null 2>&1; then \
			docker --context "$(DOCKER_CONTEXT)" volume create "$(QDRANT_VOLUME_NAME)" >/dev/null; \
		fi; \
		$(COMPOSE) up -d qdrant; \
		for i in {1..100}; do \
			if curl --connect-timeout 1 --max-time 2 -sf "$(QDRANT_URL)/collections" >/dev/null 2>&1; then \
				echo "qdrant reachable at $(QDRANT_URL)"; \
				exit 0; \
			fi; \
			sleep 0.2; \
		done; \
		echo "qdrant did not become reachable at $(QDRANT_URL)" >&2; \
		exit 1; \
	fi

qdrant-down:
	$(COMPOSE) stop qdrant

qdrant-status:
	curl -sf $(QDRANT_URL)/collections | jq .

default-status: qdrant-up
	@set -euo pipefail; \
	if ! curl -sf $(QDRANT_URL)/collections/personal_docs >/dev/null 2>&1; then \
		echo "collection personal_docs missing"; \
		exit 1; \
	fi; \
	curl -sf -X POST $(QDRANT_URL)/collections/personal_docs/points/count \
		-H "Content-Type: application/json" \
		-d '{"exact":true}' | jq '.result.count'

chatgpt-doctor: qdrant-up
	@set -euo pipefail; \
	tmp_file="$$(mktemp)"; \
	trap 'rm -f "$$tmp_file"' EXIT; \
	echo "qdrant=$(QDRANT_URL) ok"; \
	curl -sf $(OLLAMA_URL)/api/tags >"$$tmp_file"; \
	echo "ollama=$(OLLAMA_URL) ok"; \
	if jq -e --arg model "$(OLLAMA_MODEL)" '.models[] | select(.name == $$model)' "$$tmp_file" >/dev/null; then \
		echo "ollama_model=$(OLLAMA_MODEL) ok"; \
	else \
		echo "ollama_model=$(OLLAMA_MODEL) missing"; \
		exit 1; \
	fi; \
	if curl -sf $(QDRANT_URL)/collections/$(CHATGPT_COLLECTION) >/dev/null 2>&1; then \
		count=$$(curl -sf -X POST $(QDRANT_URL)/collections/$(CHATGPT_COLLECTION)/points/count -H "Content-Type: application/json" -d '{"exact":true}' | jq '.result.count'); \
		echo "collection=$(CHATGPT_COLLECTION) count=$$count"; \
	else \
		echo "collection=$(CHATGPT_COLLECTION) missing"; \
	fi

chatgpt-ingest: qdrant-up
	@test -n "$(EXPORT)" || (echo "EXPORT=/path/to/chatgpt-export.json-or-zip required" && exit 1)
	go run ./cmd/ingest_chatgpt \
		--export "$(EXPORT)" \
		--qdrant $(QDRANT_URL) \
		--collection $(CHATGPT_COLLECTION) \
		--embedder $(CHATGPT_EMBEDDER) \
		--ollama_url $(OLLAMA_URL) \
		--ollama_model $(OLLAMA_MODEL) \
		--dim $(CHATGPT_DIM) \
		--batch $(CHATGPT_BATCH) \
		--report-out "$(REPORT_OUT)"

chatgpt-server: qdrant-up
	go run ./cmd/server \
		--addr $(CHATGPT_SERVER_ADDR) \
		--qdrant $(QDRANT_URL) \
		--collection $(CHATGPT_COLLECTION) \
		--embedder $(CHATGPT_EMBEDDER) \
		--ollama_url $(OLLAMA_URL) \
		--ollama_model $(OLLAMA_MODEL) \
		--dim $(CHATGPT_DIM)

chatgpt-search:
	@test -n "$(QUERY)" || (echo "QUERY='...'" && exit 1)
	go run ./cmd/search_chatgpt \
		--server $(CHATGPT_SERVER_URL) \
		--format $(CHATGPT_FORMAT) \
		--query "$(QUERY)" \
		$(CHATGPT_SEARCH_ARGS)

chatgpt-mcp: mcp

mcp:
	go run ./cmd/mcp_search \
		--server $(CHATGPT_SERVER_URL)

chatgpt-status: qdrant-up
	@set -euo pipefail; \
	if ! curl -sf $(QDRANT_URL)/collections/$(CHATGPT_COLLECTION) >/dev/null 2>&1; then \
		echo "collection $(CHATGPT_COLLECTION) missing"; \
		exit 1; \
	fi; \
	curl -sf -X POST $(QDRANT_URL)/collections/$(CHATGPT_COLLECTION)/points/count \
		-H "Content-Type: application/json" \
		-d '{"exact":true}' | jq '.result.count'

smoke: qdrant-up
	@set -euo pipefail; \
	collection="smoke_$$(date +%s)"; \
	echo "collection=$$collection"; \
	go run ./cmd/ingest_chatgpt \
		--export internal/integration/testdata/chatgpt_export_valid.json \
		--qdrant $(QDRANT_URL) \
		--collection "$$collection" \
		--embedder fake \
		--dim 16 \
		--batch 2; \
	go run ./cmd/server \
		--addr 127.0.0.1:18080 \
		--qdrant $(QDRANT_URL) \
		--collection "$$collection" \
		--embedder fake \
		--dim 16 >/tmp/personal-search-smoke-server.log 2>&1 & \
	pid=$$!; \
	trap 'kill $$pid >/dev/null 2>&1 || true' EXIT; \
	for i in {1..50}; do \
		code=$$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:18080/health || true); \
		if [ "$$code" = "200" ]; then break; fi; \
		sleep 0.1; \
	done; \
	curl -s http://127.0.0.1:18080/search \
		-H "Content-Type: application/json" \
		-d '{"query":"custody strategy planning notes","limit":3}' | jq .

acceptance: qdrant-up
	go test ./internal/integration -run TestAcceptance_E2E -v -count=1

integration: qdrant-up
	go test ./internal/integration -v -count=1

semantic-smoke: qdrant-up
	bash scripts/semantic_smoke_compare.sh

# Explicit private-data check; no services started and no indexing performed.
# Environment/command-line path overrides the trusted local shell config.
chatgpt-snapshot:
	@set -euo pipefail; \
	snapshot_export="$${CHATGPT_EXPORT_PATH:-}"; \
	if [[ -z "$$snapshot_export" && -f .env.local ]]; then \
		snapshot_export="$$( . ./.env.local; printf '%s' "$${CHATGPT_EXPORT_PATH:-}" )"; \
	fi; \
	if [[ -z "$$snapshot_export" ]]; then \
		echo "Set CHATGPT_EXPORT_PATH in .env.local or the environment" >&2; exit 1; \
	fi; \
	CHATGPT_EXPORT_PATH="$$snapshot_export" go test ./internal/chatgpt -run '^TestSnapshot_RealExport$$' -v -count=1
