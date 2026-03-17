SHELL := /bin/bash

QDRANT_URL ?= http://localhost:6333
CHATGPT_COLLECTION ?= chatgpt_messages
CHATGPT_SERVER_ADDR ?= 127.0.0.1:18080
CHATGPT_SERVER_URL ?= http://$(CHATGPT_SERVER_ADDR)
CHATGPT_EMBEDDER ?= ollama
OLLAMA_URL ?= http://localhost:11434
OLLAMA_MODEL ?= nomic-embed-text:latest
CHATGPT_DIM ?= 0
CHATGPT_BATCH ?= 8
CHATGPT_FORMAT ?= json

.PHONY: qdrant-up qdrant-down qdrant-status default-status chatgpt-doctor chatgpt-ingest chatgpt-server chatgpt-search chatgpt-mcp chatgpt-status smoke acceptance integration semantic-smoke

qdrant-up:
	@set -euo pipefail; \
	if curl -sf $(QDRANT_URL)/collections >/dev/null 2>&1; then \
		echo "qdrant already reachable at $(QDRANT_URL)"; \
	else \
		docker compose up -d qdrant; \
	fi

qdrant-down:
	docker compose down

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
	echo "qdrant=$(QDRANT_URL) ok"; \
	curl -sf $(OLLAMA_URL)/api/tags >/tmp/chatgpt-ollama-tags.json; \
	echo "ollama=$(OLLAMA_URL) ok"; \
	if jq -e --arg model "$(OLLAMA_MODEL)" '.models[] | select(.name == $$model)' /tmp/chatgpt-ollama-tags.json >/dev/null; then \
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
		--batch $(CHATGPT_BATCH)

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
		--query "$(QUERY)"

chatgpt-mcp:
	go run ./cmd/mcp_chatgpt_search \
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
