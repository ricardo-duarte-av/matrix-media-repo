# Offline homeserver importers

These tools read local media **directly from a homeserver's media store on disk** (using its Postgres
database only to enumerate records and metadata) and write an **MMR v2 archive** — the same package
format produced by GDPR exports. That archive is then imported into MMR through the standard admin
import API.

| Tool | Source homeserver | Media store layout |
| --- | --- | --- |
| `export_synapse_for_import` | Synapse | `media_store_path/local_content/AA/BB/CCDD` (+ `url_cache/...`) |
| `export_dendrite_for_import` | Dendrite | `base_path/A/B/CCDD/file` |

Unlike the [live importers](../homeserver_live_importers), these do **not** call the homeserver's
media API, so they work even when unauthenticated media access is disabled — but they need direct
filesystem access to the media store. To go the other direction (MMR → homeserver), see
[`../homeserver_offline_exporters`](../homeserver_offline_exporters).

## What it does

1. Connects to the homeserver's Postgres DB and reads every `local_media_repository` record.
2. For each record, resolves the file's on-disk path and streams it into a chunked archive
   (`.tgz` parts of roughly `-partSize` bytes) written to `-destination`.
3. Leaves the homeserver completely untouched (read-only against DB and files).

### Importing the archive into MMR

The output directory contains the archive parts. Import them into MMR the same way as any v2/GDPR
archive, via the admin import API on the background-tasks worker:

```
POST /_matrix/media/unstable/admin/import          -> returns import_id, send first part as body
POST /_matrix/media/unstable/admin/import/:id/part -> send each additional part
POST /_matrix/media/unstable/admin/import/:id/close
```

See the admin documentation for details.

## Usage

```sh
./export_synapse_for_import \
    -serverName example.org \
    -mediaDirectory /var/lib/synapse/media_store \
    -destination ./media-export \
    -dbHost localhost -dbPort 5432 \
    -dbUsername synapse -dbName synapse \
    -partSize 104857600
```

From source, use `go run ./cmd/homeserver_offline_importers/export_synapse_for_import`.
`export_dendrite_for_import` takes the same flags (point `-mediaDirectory` at Dendrite's
`media_api.base_path`).

### Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `-serverName` | `localhost` | Your homeserver name. |
| `-mediaDirectory` | `./media_store` | Path to the homeserver's on-disk media store. |
| `-destination` | `./media-export` | Output directory for the archive (created if needed). |
| `-partSize` | `104857600` (100 MiB) | Approx. size to split archive parts into. |
| `-skipMissing` | `false` | Skip records whose file is missing instead of aborting. **See caveat.** |
| `-dbHost` / `-dbPort` | `localhost` / `5432` | Homeserver Postgres host/port. |
| `-dbUsername` / `-dbName` | software name | Homeserver Postgres user/database. |
| `-dbPassword` | *(prompted)* | Homeserver Postgres password. Omit to be prompted. |
| `-templates` | built-in | Path to MMR templates folder. |
| `-debug` / `-prettyLog` | `false` | Verbose / coloured logging. |

## Caveats & gotchas

- **`-skipMissing`** skips records whose media file is missing and reports them at the end. Without
  it, the first missing file aborts the export.
- **Disk space:** the archive is written in full to `-destination`; ensure you have room for a second
  copy of the media (minus dedup/compression).
- **`-dbPassword` on the command line** is visible in shell history and the process list; prefer the
  prompt.
- **TLS:** the Postgres connection uses `sslmode=disable`; run it local to the database.
- Read-only against the source homeserver — safe to run while the homeserver is live, though a
  quiescent media store gives the most consistent snapshot.
