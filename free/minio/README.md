# minio

A free compatibility plugin. It runs MinIO as the `minio` service on the `minio_data` volume, the same way core did in v1.4. Use it only when you already have data in MinIO and want it to keep working under v1.5. New projects should use the core SeaweedFS store (`OBJECT_STORAGE_ENABLED=true`, see the Object-Storage page in the CLI wiki).

## When to use it

Use it if all of these are true:

- Your project ran with `MINIO_ENABLED=true` in v1.4, and the `<project>_minio_data` volume holds objects you need.
- You are on a CLI in v1.5 mode (`NSELF_V15=1`, the default from v1.5.0).
- You have not moved those objects to another store yet.

In v1.5 mode a project that wants MinIO without this plugin stops with **E138** before any file changes. The message names three ways out. This plugin is the first one.

Do not install it if the project never used MinIO.

Plugins are machine-wide. `nself build` adds this fragment to every project built on the machine, whether or not the project declares MinIO. Only the `S3_*` variables for custom services follow MinIO intent (`MINIO_ENABLED=true`).

So a project without MinIO still gets its own empty `<project>_minio` container on the same loopback ports, and two such projects running at once clash on port 9000.

`nself plugin disable minio` takes effect on the next `nself build`, but it also applies to every project on the machine. Keep MinIO projects and other projects on separate machines, or give each project its own `MINIO_PORT` and `MINIO_CONSOLE_PORT`.

## Data continuity

Installing the plugin is not a data migration. Nothing is copied, converted or rewritten.

- The service name is `minio`, the container name is `<project>_minio`, and the volume is `minio_data`. Compose names the volume `<project>_minio_data`, which is the volume v1.4 created. It attaches with its data.
- The root user and password come from `MINIO_ROOT_USER` and `MINIO_ROOT_PASSWORD`, the same keys core used. When a key is unset the fragment uses the core default (`minioadmin`, and the core bucket list `uploads,public,private,temp`). Keep the credentials the data was created with.
- Ports are the core ones: S3 on `127.0.0.1:${MINIO_PORT:-9000}`, console on `127.0.0.1:${MINIO_CONSOLE_PORT:-9001}`. Both bind loopback only.
- The container keeps the core hardening: read-only root, all capabilities dropped except `CHOWN`, `SETUID` and `SETGID`, `no-new-privileges`, a 512m memory limit. No Docker socket, no privileged mode.
- Keep `MINIO_ENABLED=true` in `.env` after you install the plugin. That setting tells the project to use the plugin's store. While core object storage is off, custom services get `S3_ENDPOINT=http://minio:9000`. Once `OBJECT_STORAGE_ENABLED=true` is set they get the SeaweedFS endpoint instead (see the move below).
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
2. **Run both stores.** In v1.5 mode keep the plugin installed with `MINIO_ENABLED=true`, and add `OBJECT_STORAGE_ENABLED=true` plus `S3_ACCESS_KEY` and `S3_SECRET_KEY` in `.env.secrets`. SeaweedFS starts with its own empty `seaweedfs_data` volume. Run `nself build` and `nself start`. From this build on, custom services get `S3_ENDPOINT=http://seaweedfs:8333`, which is still empty. Pause writes in your applications until the switch, or accept that new objects land in SeaweedFS while old ones are still in MinIO.
3. **Copy.** Run `mc` in a one-off container on the project network, so `minio:9000` and `seaweedfs:8333` resolve. Give it the two endpoints through a `0600` env file, so no key appears on a command line:

   ```bash
   # mirror.env (chmod 600), one line per store, no quotes:
   #   MC_HOST_old=http://<MINIO_ROOT_USER>:<MINIO_ROOT_PASSWORD>@minio:9000
   #   MC_HOST_new=http://<S3_ACCESS_KEY>:<S3_SECRET_KEY>@seaweedfs:8333
   docker run --rm --network <project>_network --env-file mirror.env \
     --entrypoint mc docker.io/pgsty/minio:RELEASE.2026-08-04T00-00-00Z \
     mirror old/<bucket> new/<bucket>
   ```

   Each line is a URL, so percent-encode special characters in a key or password (`/` becomes `%2F`, `+` becomes `%2B`, `@` becomes `%40`). Repeat for every bucket. Delete `mirror.env` when you are done.
4. **Verify.** For each bucket, compare the object count and the total size on both sides (`mc ls --recursive` and `mc du`). Spot-check a few objects by checksum. Test your application against the SeaweedFS endpoint. Recreate bucket policies and any CORS settings by hand: they do not travel with `mc mirror`.
5. **Finish the switch.** Custom services already read the SeaweedFS endpoint since step 2. Point any application that talks to MinIO directly at `http://seaweedfs:8333`, then set `MINIO_ENABLED=false` in `.env`. The `MINIO_ROOT_*` keys alone still read as MinIO intent, so keep `MINIO_ENABLED=false` explicit.
6. **Remove the plugin.** `nself remove minio`, then `nself build`. The `minio_data` volume is left on disk. When you are sure the copy is good, delete it yourself with `docker volume rm <project>_minio_data`.

## Known gaps

- MinIO community editions have no bucket CORS API. The only CORS control is the server-wide `MINIO_API_CORS_ALLOW_ORIGIN` setting, which this plugin does not set. SeaweedFS in core does support bucket CORS.
- The plugin runs the `pgsty/minio` build. It receives fixes only when the pinned image is updated here.
- The fragment is not gated on MinIO intent: every project built on a machine with the plugin gets a `minio` service (see "When to use it"). Gating it the way the `S3_*` variables are gated is CLI work.
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
NSELF=/path/to/nself/checkout TMPDIR=/path/to/scratch bash free/minio/test/migrate-from-core.sh
```

Needs Go, openssl and a Docker daemon that runs Linux containers and allows a privileged `docker:dind` sibling. It never installs anything on your machine; inside a root CI container, `MINIO_E2E_INSTALL_DOCKER=1` lets it install the docker client. Exit codes: 0 pass, 1 fail, 2 usage, 3 missing precondition.
