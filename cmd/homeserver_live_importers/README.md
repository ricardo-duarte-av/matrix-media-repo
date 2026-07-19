# Live homeserver importers

These tools copy local media **out of a running homeserver and into MMR**, by reading the
homeserver's PostgreSQL database to enumerate media and then downloading each item over the
homeserver's client-server media API.

| Tool | Source homeserver |
| --- | --- |
| `import_synapse` | Synapse |
| `import_dendrite` | Dendrite |

For the archive-based (no direct MMR DB access) alternative, see
[`../homeserver_offline_importers`](../homeserver_offline_importers). To go the other direction
(MMR → homeserver), see [`../homeserver_offline_exporters`](../homeserver_offline_exporters).

## What it does

1. Connects to the homeserver's Postgres DB and reads every `local_media_repository` record.
2. For each record, checks MMR's own database — **already-imported media is skipped**, so the tool
   is safe to re-run/resume.
3. Downloads the bytes from `GET {baseUrl}/_matrix/media/v3/download/{serverName}/{mediaId}` and
   uploads them into MMR via the normal upload pipeline.
4. Warns on any size mismatch between the DB record and the downloaded bytes.

This writes directly into MMR's configured datastore and database. It does **not** modify the source
homeserver.

## Requirements

- Read access to the homeserver's Postgres database.
- The homeserver's media API reachable at `-baseUrl` and able to serve the media **unauthenticated**
  (see caveats).
- A working MMR config (`-config`) pointing at MMR's database and datastore, plus the migrations
  folder (`-migrations`).

## Usage

From a release build:

```sh
./import_synapse \
    -serverName example.org \
    -baseUrl https://example.org \
    -dbHost localhost -dbPort 5432 \
    -dbUsername synapse -dbName synapse \
    -config /etc/media-repo.yaml \
    -migrations ./migrations \
    -workers 10
```

From source, swap the binary for `go run ./cmd/homeserver_live_importers/import_synapse`.
`import_dendrite` takes the same flags.

### Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `-serverName` | `localhost` | Your homeserver name (e.g. `example.org`). |
| `-baseUrl` | `http://localhost:8008` | Base URL of the homeserver's media API. |
| `-dbHost` / `-dbPort` | `localhost` / `5432` | Homeserver Postgres host/port. |
| `-dbUsername` / `-dbName` | software name | Homeserver Postgres user/database. |
| `-dbPassword` | *(prompted)* | Homeserver Postgres password. Omit to be prompted (preferred — see caveats). |
| `-config` | `media-repo.yaml` | Path to MMR config. Overridden by `REPO_CONFIG` env var if set. |
| `-migrations` | `./migrations` | Path to MMR's migrations folder. |
| `-workers` | `10` | Concurrent download workers. |

## Caveats & gotchas

- **Authenticated media:** the download uses the legacy *unauthenticated* `/_matrix/media/v3/download`
  endpoint with no access token. Homeservers that have disabled unauthenticated media access will
  reject these requests. Run the import before locking media down, or use the offline importer, which
  reads files from disk instead.
- **Fail-fast:** any single failed download or DB error aborts the whole run (the tool panics). It is
  safe to re-run — completed media is skipped — but there is no automatic retry, and a single broken
  media record will stop the batch until resolved or removed.
- **No download timeout:** a stalled connection to the homeserver can hang a worker indefinitely. If a
  run appears stuck, interrupt and re-run.
- **`-dbPassword` on the command line** is visible in your shell history and the process list. Prefer
  omitting it and entering it at the prompt.
- **TLS:** the Postgres connection is made with `sslmode=disable`. Run it on the same host/private
  network as the database; don't point it across an untrusted network.
