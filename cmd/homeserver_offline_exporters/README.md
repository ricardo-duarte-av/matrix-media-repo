# Offline homeserver exporters

This tool goes the opposite direction from the importers: it takes an **MMR v2 archive** (a GDPR
export, or the output of the [offline importers](../homeserver_offline_importers)) and writes the
media **into a homeserver's media store and database**, so you can migrate *off* MMR back onto the
homeserver's native media handling.

| Tool | Target homeserver |
| --- | --- |
| `import_to_synapse` | Synapse |

(There is currently no Dendrite target.)

## What it does

For each media record in the archive:

1. Checks the target Synapse DB — **already-present media is skipped**, so it is safe to re-run.
2. Writes the file into Synapse's layout: `media_store_path/local_content/AA/BB/CCDD`.
3. Inserts a `local_media_repository` row for it.
4. Optionally (`-generateThumbnails`) pre-generates Synapse's standard thumbnail sizes
   (32×32 crop, 96×96 crop, 320×240/640×480/800×600 scale) and records them. Thumbnail generation is
   best-effort: individual failures are logged and skipped, not fatal.

## ⚠️ This writes to a live homeserver

Unlike the importers (which only read from the homeserver), this tool **inserts rows into Synapse's
database and writes into its media store**. Treat it as a destructive/administrative operation:

- **Back up the Synapse database and media store first.**
- Ideally run it while **Synapse is stopped**, then start Synapse afterwards.
- Large media (> 2 GiB / `MaxInt32`) is flagged with a warning — Synapse historically mishandles these
  (matrix-org/synapse#12023).

## Usage

```sh
./import_to_synapse \
    -serverName example.org \
    -directory ./media-export \
    -mediaDirectory /var/lib/synapse/media_store \
    -dbHost localhost -dbPort 5432 \
    -dbUsername synapse -dbName synapse \
    -generateThumbnails
```

From source: `go run ./cmd/homeserver_offline_exporters/import_to_synapse`.

### Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `-serverName` | `localhost` | Your homeserver name. |
| `-directory` | `./gdpr-data` | Directory containing the MMR archive parts to import. |
| `-mediaDirectory` | `./media_store` | Target Synapse media store path. |
| `-generateThumbnails` | `false` | Also pre-populate Synapse's thumbnail cache. |
| `-dbHost` / `-dbPort` | `localhost` / `5432` | Synapse Postgres host/port. |
| `-dbUsername` / `-dbName` | `synapse` | Synapse Postgres user/database. |
| `-dbPassword` | *(prompted)* | Synapse Postgres password. Omit to be prompted. |
| `-debug` / `-prettyLog` | `false` | Verbose / coloured logging. |

## Caveats & gotchas

- **Only local media / `local_content` is written** — the tool does not populate remote media caches.
- **No transaction across file + DB:** a crash mid-record can leave a file on disk without its DB row
  (or vice versa). Re-running skips media already in the DB; orphaned files are harmless but not
  cleaned up.
- **`-dbPassword` on the command line** is visible in shell history and the process list; prefer the
  prompt.
- **TLS:** the Postgres connection uses `sslmode=disable`; run it local to the database.
- The closing log line ("If there's no warnings above, you're probably fine.") is literal — scan the
  output for warnings before trusting the migration.
