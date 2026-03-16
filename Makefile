SHELL := /bin/bash

QDRANT_URL ?= http://localhost:6333

.PHONY: qdrant-up qdrant-down qdrant-status default-status smoke acceptance integration semantic-smoke

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
