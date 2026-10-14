# Local runtime: Socktainer and Docker Compose

Qdrant uses the committed `docker-compose.yml`. On Apple Silicon macOS 26+, [Socktainer](https://github.com/socktainer/socktainer) provides the Docker API. The Go server and Ollama run on the host.

## Install

With [Homebrew](https://brew.sh/) installed:

```sh
brew install socktainer
```

Homebrew installs the compatible Container runtime as a dependency. Socktainer 1.5.3 requires Container 1.5.0; an older runtime selected by another app must be stopped before starting Socktainer. Keep the backend version compatible with the installed Socktainer release.

Use existing Docker CLI and Compose installations, or install them:

```sh
brew install docker docker-compose
docker compose version
```

If Docker cannot find Homebrew's Compose plugin, add the directory printed by `brew info docker-compose` to the `cliPluginsExtraDirs` array in `~/.docker/config.json`, preserving other settings. For Apple Silicon Homebrew:

```json
"cliPluginsExtraDirs": ["/opt/homebrew/lib/docker/cli-plugins"]
```

## Start

In a separate terminal, run as your normal user and leave it running:

```sh
socktainer --volume-sync=fsync
```

Socktainer automatically starts its backend and registers a Docker context named `socktainer` at `~/.socktainer/container.sock`. The `fsync` setting honors database flushes for named volumes. These instructions use the current user's home and data; Homebrew's service uses a different home/socket, so it is not interchangeable with this startup command.

From the repository root:

```sh
docker --context socktainer info
make qdrant-up
./chatgpt-conversation-search doctor
./chatgpt-conversation-search server
```

Open <http://127.0.0.1:18080/>. For the populated Voice Memos index on this machine, use <http://127.0.0.1:18080/?source=voicememo>.

The Makefile selects `socktainer` explicitly; it does not change your global Docker context. A reachable `QDRANT_URL` is reused without starting another database. Startup failures propagate instead of being ignored.

## Database persistence

Compose pins Qdrant to `qdrant/qdrant:v1.19.2`, publishes ports `6333` and `6334` on loopback, and mounts `personal-search-qdrant-data` at `/qdrant/storage`. `make qdrant-up` creates this volume only when missing. It is external to the Compose lifecycle, so container recreation and Compose teardown preserve the index.

Stop only the project's database:

```sh
make qdrant-down
```

For direct Compose commands, create the external volume once first:

```sh
docker --context socktainer volume create personal-search-qdrant-data
docker --context socktainer compose up -d qdrant
```

`QDRANT_IMAGE` and `QDRANT_VOLUME_NAME` can be overridden through the environment or Make command line. Change database versions deliberately; do not recreate an existing index with an arbitrary `latest` image.

## Existing database and other Docker backends

Socktainer sees the existing backend's named volumes. On this machine, the previous manual container `personal-search-qdrant` must stay stopped while Compose runs `personal-search-qdrant-1`: both reference the same database volume. When adopting a manual container, stop it before `make qdrant-up`:

```sh
docker --context socktainer stop personal-search-qdrant
make qdrant-up
```

For another Docker backend, use its context for every lifecycle command:

```sh
DOCKER_CONTEXT=desktop-linux make qdrant-up
DOCKER_CONTEXT=desktop-linux make qdrant-down
```

Separate Docker backends have separate volumes; selecting another context does not migrate data. Stop the existing database before starting another on the same host ports.

Earlier Docker Compose setups used the volume `personal-search_qdrant_data`. To reuse that database, inspect its volume and Qdrant version first, then set `QDRANT_VOLUME_NAME=personal-search_qdrant_data` with the matching Docker context. The new default volume name does not automatically select that older database.

## Index coverage

The restored volume contained 129 successful cached Voice Memo transcripts on 2026-10-14. Cache coverage ended June 24, 2026; 45 stale and 3 empty transcripts were skipped. This volume is not a migration of the prior Docker database. The configured ChatGPT export path no longer exists; ChatGPT, Notes, and Google indexes remain unpopulated. Import those sources before using All sources. No automatic login/startup job is installed by this project.

Verified 2026-10-14: Socktainer 1.5.3 with Homebrew Container 1.5.0; Compose created the database container using the existing volume; `make qdrant-down` / `make qdrant-up` preserved all 129 voice transcripts; real `/search` requests returned Voice Memo results. The previous manual database container remains stopped. The containerized API overlay was validated as Compose configuration, but its build and host Ollama networking were not exercised in this migration.
