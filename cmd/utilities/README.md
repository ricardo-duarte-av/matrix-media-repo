# Signing key utilities

MMR supports [MSC3916](https://github.com/matrix-org/matrix-spec-proposals/pull/3916)-style
authenticated media. Serving **inbound** authenticated-media requests needs no signing key, but
authorizing **outbound** federated media requests (fetching remote media on behalf of your users)
requires a signing key that your homeserver also trusts.

This directory contains the two tools used to set that up:

| Tool | Purpose |
| --- | --- |
| [`generate_signing_key`](./generate_signing_key) | Create a new signing key for MMR, or convert an existing key between formats. |
| [`combine_signing_keys`](./combine_signing_keys) | Merge several signing keys into a single file for homeservers that support multiple keys. |

> **Security warning:** a signing key effectively grants full access to your server and its events.
> Treat it like a private credential — never commit it, share it, or paste it anywhere. Back up your
> existing homeserver key before you start.

## Getting the tools

Download the `generate_signing_key` and `combine_signing_keys` binaries for your MMR version from
the [GitHub releases page](https://github.com/t2bot/matrix-media-repo/releases). They ship with the
Docker image too, on `PATH`.

To run them from a source checkout instead, use `go run`:

```sh
go run ./cmd/utilities/generate_signing_key -help
go run ./cmd/utilities/combine_signing_keys -help
```

The examples below assume compiled binaries in the current directory (`./generate_signing_key`).
Substitute `go run ./cmd/utilities/generate_signing_key` if working from source.

## Flags

### `generate_signing_key`

| Flag | Default | Meaning |
| --- | --- | --- |
| `-format` | `mmr` | Output format: `mmr`, `synapse`, or `dendrite`. |
| `-output` | `./signing.key` | File to write the key to. |
| `-input` | *(none)* | If set, convert this existing key to `-format` instead of generating a new one. Input format is auto-detected. If the input holds multiple keys, only the first is converted. |

### `combine_signing_keys`

| Flag | Default | Meaning |
| --- | --- | --- |
| `-format` | `mmr` | Output format: `mmr`, `synapse`, or `dendrite`. |
| `-output` | `./signing.key` | File to write the combined keys to. |

Keys to combine are passed as positional arguments **in order** — the order is preserved in the
output file. Supplying two files that share a key version is an error.

## How-to (Synapse)

This walks through adding an MMR signing key to a Synapse homeserver. For other homeserver software,
consult its documentation on deploying multiple signing keys — not all of them support more than one.

### 1. Find and back up Synapse's existing signing key

Synapse's key lives at the path given by `signing_key_path` in its `homeserver.yaml` (often
`<server_name>.signing.key`). Copy it somewhere safe first:

```sh
cp /path/to/existing.signing.key /path/to/backups/existing.signing.key.bak
```

You can read your current key ID from a browser or curl:

```
GET https://<your-homeserver>/_matrix/key/v2/server
```

Note the `ed25519:<version>` key ID — you'll confirm it stays first later.

### 2. Generate MMR's own key

```sh
./generate_signing_key -output ./mmr.signing.key
```

`mmr` is the default format, so no `-format` is needed. The tool prints the new key ID
(`ed25519:<version>`).

### 3. Combine Synapse's key with MMR's key

List **Synapse's existing key first**, then MMR's, and output in Synapse format:

```sh
./combine_signing_keys -format synapse -output ./merged.signing.key \
    ./existing.signing.key \
    ./mmr.signing.key
```

Order matters: whichever key is listed first ends up on the first line of the output.

### 4. Verify the order

```sh
cat ./merged.signing.key
```

Confirm that **your existing homeserver key ID is on the first line** (compare against the key ID
from `/_matrix/key/v2/server` in step 1). If it isn't, re-run step 3 with the arguments in the
correct order.

### 5. Deploy the merged key to Synapse

Replace Synapse's signing key with `merged.signing.key` (i.e. point `signing_key_path` at it, or
overwrite the existing file) and restart Synapse. Synapse now advertises both keys, so it will
accept requests signed by either.

### 6. Deploy MMR's key to MMR

Deploy `mmr.signing.key` alongside MMR and reference it in your MMR config under the relevant
homeserver's `signingKeyPath`:

```yaml
homeservers:
  - name: example.org
    # ...
    signingKeyPath: "/data/mmr.signing.key"
```

Restart MMR. Outbound federated media requests are now signed with a key Synapse trusts.

## Revoking or rotating

- **To revoke MMR's key:** restore your homeserver's signing key from the backup taken in step 1 and
  restart the homeserver.
- **If your homeserver's signing key changes:** re-run this whole process with the new key.
- **Good practice:** list old/revoked keys (including MMR's) under `old_verify_keys` on
  `/_matrix/key/v2/server` so previously-signed material still validates. Many homeservers offer a
  config option for this.

## See also

- MMR signing key docs: <https://docs.t2bot.io/matrix-media-repo/latest/installation/signing-key/>
