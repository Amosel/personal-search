# Local Apple Container runtime

Installed 2026-10-14 into `~/.local/opt/apple-container-1.5.0` from Apple's signed 1.5.0 installer payload. The package signature and published SHA-256 were verified. System-wide installation was unavailable without an administrator password; the CLI's supported `--install-root` points at this user-local installation.

Qdrant runs as `personal-search-qdrant`, Linux arm64 image `docker.io/qdrant/qdrant:v1.19.2`, persistent 20 GiB volume `personal-search-qdrant-data`, published only on `127.0.0.1:6333`. Go serving and Ollama remain native.

Start runtime and existing database:

```sh
"$HOME/.local/opt/apple-container-1.5.0/bin/container" system start \
  --install-root "$HOME/.local/opt/apple-container-1.5.0" --enable-kernel-install
"$HOME/.local/opt/apple-container-1.5.0/bin/container" start personal-search-qdrant
```

Start the actual API/UI from this repository, with Ollama running:

```sh
go run ./cmd/server --addr 127.0.0.1:18080 \
  --collection chatgpt_messages --embedder ollama
```

Do not use `python3 -m http.server` for this application: it cannot handle the search API's POST requests. Stop any task-owned preview occupying port 18080 before starting the API.

Open <http://127.0.0.1:18080/?source=voicememo>. The URL selects Voice Memos, which is currently populated. Source selection remains changeable in the UI.

Stop only this database, preserving its volume:

```sh
"$HOME/.local/opt/apple-container-1.5.0/bin/container" stop personal-search-qdrant
```

Verification on 2026-10-14: imported 129 successful cached voice transcripts; restarted the container; exact count remained 129; real POST search returned voice results. Cache contains recordings only through June 24, 2026; 45 stale and 3 empty transcripts were skipped. This fresh Apple volume is not a migration of the prior database. The configured ChatGPT export path no longer exists; ChatGPT remains empty here. Notes and Google collections have not been restored, so All sources is unavailable until those imports exist. No automatic login/startup job was installed.
