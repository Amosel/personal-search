SHELL := /bin/bash

QDRANT_URL ?= http://localhost:6333
QDRANT_RUNTIME ?= container
QDRANT_IMAGE ?= qdrant/qdrant:latest
QDRANT_CONTAINER_NAME ?= personal-search-qdrant
QDRANT_VOLUME_NAME ?= personal-search-qdrant-data
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
	if curl -sf $(QDRANT_URL)/collections >/dev/null 2>&1; then \
		echo "qdrant already reachable at $(QDRANT_URL)"; \
	else \
		case "$(QDRANT_RUNTIME)" in \
			container) \
				if ! command -v container >/dev/null 2>&1; then \
					echo "Apple container CLI not found. Install it, or run QDRANT_RUNTIME=docker make qdrant-up." >&2; \
					exit 1; \
				fi; \
				if ! container system status >/dev/null 2>&1; then \
					echo "Apple container services are not running. Start them with: container system start" >&2; \
					exit 1; \
				fi; \
				if ! container volume inspect $(QDRANT_VOLUME_NAME) >/dev/null 2>&1; then \
					container volume create $(QDRANT_VOLUME_NAME) >/dev/null; \
				fi; \
				if container inspect $(QDRANT_CONTAINER_NAME) >/dev/null 2>&1; then \
					container start $(QDRANT_CONTAINER_NAME) >/dev/null 2>&1 || true; \
				else \
					container run -d \
						--name $(QDRANT_CONTAINER_NAME) \
						-p 6333:6333 \
						-p 6334:6334 \
						-v $(QDRANT_VOLUME_NAME):/qdrant/storage \
						$(QDRANT_IMAGE) >/dev/null; \
				fi; \
				;; \
			docker) \
				docker compose up -d qdrant; \
				;; \
			*) \
				echo "Unsupported QDRANT_RUNTIME=$(QDRANT_RUNTIME). Use 'container' or 'docker'." >&2; \
				exit 1; \
				;; \
		esac; \
		for i in {1..100}; do \
			if curl -sf $(QDRANT_URL)/collections >/dev/null 2>&1; then \
				echo "qdrant reachable at $(QDRANT_URL)"; \
				exit 0; \
			fi; \
			sleep 0.2; \
		done; \
		echo "qdrant did not become reachable at $(QDRANT_URL)" >&2; \
		exit 1; \
	fi

qdrant-down:
	@set -euo pipefail; \
	case "$(QDRANT_RUNTIME)" in \
		container) \
			if ! command -v container >/dev/null 2>&1; then \
				echo "Apple container CLI not found. Nothing to stop." >&2; \
				exit 0; \
			fi; \
			container stop $(QDRANT_CONTAINER_NAME) >/dev/null 2>&1 || true; \
			;; \
		docker) \
			docker compose down; \
			;; \
		*) \
			echo "Unsupported QDRANT_RUNTIME=$(QDRANT_RUNTIME). Use 'container' or 'docker'." >&2; \
			exit 1; \
			;; \
	esac

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
