# API deployment

The HTTP API is the Go server in `cmd/server`. It serves `/health`, `/status`, `/search`, `/thread`, and the embedded local UI. For an online deployment, set `PERSONAL_SEARCH_API_TOKEN`; every route except `/health` then requires either `Authorization: Bearer <token>` or `X-API-Key: <token>`.

This is API-first hardening. The embedded UI is also protected by the token, but browsers do not add API headers by themselves. For a public browser UI, add a small login/session layer or put the API behind an authenticated reverse proxy that injects the token.

## Runtime dependencies

- Qdrant containing the indexed collections. Local development uses Docker Compose through Socktainer's Docker context; see [local runtime setup](local-runtime.md). `DOCKER_CONTEXT` selects another Docker backend.
- An embedding service compatible with the collection. The imported Google Search collection uses `nomic-embed-text:latest` through Ollama, so the deployed API must reach an Ollama server with that same model digest.

## Container

```sh
docker --context socktainer build -t personal-search-api .
docker --context socktainer run --rm -p 8080:8080 \
  -e ADDR=:8080 \
  -e QDRANT_URL=http://host.docker.internal:6333 \
  -e EMBEDDER=ollama \
  -e OLLAMA_URL=http://host.docker.internal:11434 \
  -e OLLAMA_MODEL=nomic-embed-text:latest \
  -e GOOGLE_COLLECTION=google_search \
  -e GOOGLE_OLLAMA_URL=http://host.docker.internal:11434 \
  -e GOOGLE_OLLAMA_MODEL=nomic-embed-text:latest \
  -e PERSONAL_SEARCH_API_TOKEN=change-me \
  personal-search-api
```

Or, with Qdrant plus the API overlay (create the external database volume once first):

```sh
docker --context socktainer volume create personal-search-qdrant-data
PERSONAL_SEARCH_API_TOKEN=change-me docker --context socktainer compose -f docker-compose.yml -f compose.api.yml up --build
```

Use the same Docker context for the volume and Compose commands. Only one database can bind host ports `6333` and `6334` at a time.

To run only the API container while using an existing Qdrant on the host:

```sh
PERSONAL_SEARCH_API_TOKEN=change-me \
QDRANT_URL=http://host.docker.internal:6333 \
docker --context socktainer compose -f compose.api.yml up --build
```

## API smoke check

```sh
curl http://localhost:8080/health
curl -H "Authorization: Bearer change-me" "http://localhost:8080/status?source=google_search"
curl -H "Authorization: Bearer change-me" \
  -H "Content-Type: application/json" \
  -d '{"query":"example","filters":{"source":["google_search"]},"limit":5}' \
  http://localhost:8080/search
```

## Deployment notes

Do not expose Qdrant or Ollama directly to the public internet. Put only the API behind HTTPS, keep `PERSONAL_SEARCH_API_TOKEN` secret, and restrict Qdrant/Ollama to a private network, VPN, or same-host loopback.
