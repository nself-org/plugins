# minio

A free compatibility plugin. It runs MinIO as the `minio` service on the `minio_data` volume, the same way core did in v1.4. Use it only when you already have data in MinIO and want it to keep working under v1.5. New projects should use the core SeaweedFS store (`OBJECT_STORAGE_ENABLED=true`, see the Object-Storage page in the CLI wiki).

## When to use it

Use it if all of these are true:

- Your project ran with `MINIO_ENABLED=true` in v1.4, and the `<project>_minio_data` volume holds objects you need.
- You are on a CLI in v1.5 mode (`NSELF_V15=1`, the default from v1.5.0).
- You have not moved those objects to another store yet.

In v1.5 mode a project that wants MinIO without this plugin stops with **E138** before any file changes. The message names three ways out. This plugin is the first one.

Do not install it if the project never used MinIO. The plugin is machine-wide, but it only does something for a project that declares MinIO intent (`MINIO_ENABLED=true`).

## Data continuity

Installing the plugin is not a data migration. Nothing is copied, converted or rewritten.

- The service name is `minio`, the container name is `<project>_minio`, and the volume is `minio_data`. Compose names the volume `<project>_minio_data`, which is the volume v1.4 created. It attaches with its data.
- The root user and password come from `MINIO_ROOT_USER` and `MINIO_ROOT_PASSWORD`, the same keys core used. When a key is unset the fragment uses the core default (`minioadmin`, and the core bucket list `uploads,public,private,temp`). Keep the credentials the data was created with.
- Ports are the core ones: S3 on `127.0.0.1:${MINIO_PORT:-9000}`, console on `127.0.0.1:${MINIO_CONSOLE_PORT:-9001}`. Both bind loopback only.
- The container keeps the core hardening: read-only root, all capabilities dropped except `CHOWN`, `SETUID` and `SETGID`, `no-new-privileges`, a 512m memory limit. No Docker socket, no privileged mode.
- Keep `MINIO_ENABLED=true` in `.env` after you install the plugin. That setting tells the project to use the plugin's store, and custom services then get `S3_ENDPOINT=http://minio:9000`.
- `MINIO_VERSION` no longer selects the image. The plugin pins one image by tag and digest (below). To change it, update the plugin.

## Install

```bash
nself add minio
nself build
nself start
```

Check that your objects are still there with your usual S3 client against `http://127.0.0.1:9000`, or with `mc` inside the container.

## Move to SeaweedFS later

Do this when you are ready to leave MinIO. Take your time. Nothing here is urgent, and the old volume is never deleted for you.

1. **Back up first.** Take a backup of the `minio_data` volume or mirror the buckets to storage you control. Do not start the move without one.
2. **Run both stores.** In v1.5 mode keep the plugin installed with `MINIO_ENABLED=true`, and add `OBJECT_STORAGE_ENABLED=true` plus `S3_ACCESS_KEY` and `S3_SECRET_KEY` in `.env.secrets`. SeaweedFS starts with its own empty `seaweedfs_data` volume. Run `nself build` and `nself start`.
3. **Copy.** From a one-off container on the project network, or from the host through `127.0.0.1:<MINIO_PORT>` and `127.0.0.1:<SEAWEEDFS_PORT>`:

   ```bash
   mc alias set old http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"
   mc alias set new http://seaweedfs:8333 "$S3_ACCESS_KEY" "$S3_SECRET_KEY"
   mc mirror old/<bucket> new/<bucket>
   ```

   Repeat for every bucket. Pass the keys through environment variables or a file, not on the command line.
4. **Verify.** For each bucket, compare the object count and the total size on both sides (`mc ls --recursive` and `mc du`). Spot-check a few objects by checksum. Test your application against the SeaweedFS endpoint. Recreate bucket policies and any CORS settings by hand: they do not travel with `mc mirror`.
5. **Switch.** Point applications at `http://seaweedfs:8333`. Set `OBJECT_STORAGE_ENABLED=true` and `MINIO_ENABLED=false` in `.env`. The `MINIO_ROOT_*` keys alone still read as MinIO intent, so keep `MINIO_ENABLED=false` explicit.
6. **Remove the plugin.** `nself remove minio`, then `nself build`. The `minio_data` volume is left on disk. When you are sure the copy is good, delete it yourself with `docker volume rm <project>_minio_data`.

## Known gaps

- MinIO community editions have no bucket CORS API. The only CORS control is the server-wide `MINIO_API_CORS_ALLOW_ORIGIN` setting, which this plugin does not set. SeaweedFS in core does support bucket CORS.
- The plugin runs the `pgsty/minio` build. It receives fixes only when the pinned image is updated here.
- The image tag is pinned by digest. If a project needs a different image, that is a plugin update, not a `.env` setting.

## Image and licences

| Part | Licence |
|---|---|
| This plugin (manifest, compose fragment, scripts) | MIT |
| Image `docker.io/pgsty/minio` | AGPL-3.0, upstream <https://github.com/pgsty/minio> |

The image is pulled at start. It is not redistributed in this repository.

## Test

`test/migrate-from-core.sh` is the Linux proof. It builds the CLI from a checkout, creates a v1.4-mode project with `MINIO_ENABLED=true`, stores an object, then switches to v1.5 mode, installs this plugin from a local archive and checks that the same volume is mounted, the pinned image runs and the object reads back with the same checksum. It also fails when the fragment's image digest differs from the CLI image lock.

```bash
NSELF=/path/to/nself/checkout bash free/minio/test/migrate-from-core.sh
```

Needs Go, openssl and Docker. Exit codes: 0 pass, 1 fail, 2 usage, 3 missing precondition.
