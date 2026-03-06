#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${CHATGPT_EXPORT_PATH:-}" ]]; then
  echo "CHATGPT_EXPORT_PATH is required"
  exit 1
fi

if [[ -z "${OPENAI_API_KEY:-}" ]]; then
  echo "OPENAI_API_KEY is required"
  exit 1
fi

QDRANT_URL="${QDRANT_URL:-http://localhost:6333}"
MAX_DOCS="${MAX_DOCS:-500}"
MODEL="${MODEL:-text-embedding-3-small}"
DIM="${DIM:-1536}"
FAKE_DIM=16

ts="$(date +%s)"
fake_collection="smoke_fake_${ts}"
openai_collection="smoke_openai_${ts}"

echo "QDRANT_URL=${QDRANT_URL}"
echo "MAX_DOCS=${MAX_DOCS}"
echo "FAKE_COLLECTION=${fake_collection}"
echo "OPENAI_COLLECTION=${openai_collection}"

go run ./cmd/ingest_chatgpt \
  --export "${CHATGPT_EXPORT_PATH}" \
  --qdrant "${QDRANT_URL}" \
  --collection "${fake_collection}" \
  --embedder fake \
  --dim "${FAKE_DIM}" \
  --max_docs "${MAX_DOCS}" \
  --batch 128

go run ./cmd/ingest_chatgpt \
  --export "${CHATGPT_EXPORT_PATH}" \
  --qdrant "${QDRANT_URL}" \
  --collection "${openai_collection}" \
  --embedder openai \
  --openai_key "${OPENAI_API_KEY}" \
  --model "${MODEL}" \
  --dim "${DIM}" \
  --max_docs "${MAX_DOCS}" \
  --batch 64

cleanup() {
  [[ -n "${fake_pid:-}" ]] && kill "${fake_pid}" >/dev/null 2>&1 || true
  [[ -n "${openai_pid:-}" ]] && kill "${openai_pid}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

go run ./cmd/server \
  --addr 127.0.0.1:18090 \
  --qdrant "${QDRANT_URL}" \
  --collection "${fake_collection}" \
  --embedder fake \
  --dim "${FAKE_DIM}" >/tmp/personal-search-fake-server.log 2>&1 &
fake_pid=$!

go run ./cmd/server \
  --addr 127.0.0.1:18091 \
  --qdrant "${QDRANT_URL}" \
  --collection "${openai_collection}" \
  --embedder openai \
  --openai_key "${OPENAI_API_KEY}" \
  --model "${MODEL}" \
  --dim "${DIM}" >/tmp/personal-search-openai-server.log 2>&1 &
openai_pid=$!

wait_health() {
  local url="$1"
  for _ in {1..80}; do
    code=$(curl -s -o /dev/null -w "%{http_code}" "$url" || true)
    if [[ "$code" == "200" ]]; then
      return 0
    fi
    sleep 0.15
  done
  return 1
}

wait_health "http://127.0.0.1:18090/health"
wait_health "http://127.0.0.1:18091/health"

queries=(
  "custody strategy"
  "parental alienation"
  "postgres sql migration"
  "business idea"
)

for q in "${queries[@]}"; do
  echo
  echo "=== QUERY: ${q}"
  echo "--- fake"
  curl -s http://127.0.0.1:18090/search \
    -H "Content-Type: application/json" \
    -d "{\"query\":\"${q}\",\"limit\":3}" |
    jq -c '.results[] | {score, source, text:(.text|tostring|.[0:100]), thread_id:.metadata.thread_id}'

  echo "--- openai"
  curl -s http://127.0.0.1:18091/search \
    -H "Content-Type: application/json" \
    -d "{\"query\":\"${q}\",\"limit\":3}" |
    jq -c '.results[] | {score, source, text:(.text|tostring|.[0:100]), thread_id:.metadata.thread_id}'
done

echo
echo "Done. Collections:"
echo "  ${fake_collection}"
echo "  ${openai_collection}"
