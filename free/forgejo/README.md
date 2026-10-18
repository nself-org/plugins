# nself-forgejo

Self-hosted Forgejo git forge with Forgejo Actions enabled for external runners. No in-stack runner and no Docker socket mount; register runners on a CI node that is not a deploy host. Designed for the ops profile (ops server on staging/prod).

Port 3844.

## Install

```bash
nself install forgejo
```

Then rebuild so the service is wired into your stack:

```bash
nself build && nself start
```

## What it is for

A git forge and Actions control plane on hardware you own. The stack holds the
forge, its data volume and a one-shot init container that creates the admin user.
It does not run workflow jobs itself.

Self-service sign-up is closed by default. Set `FORGEJO_DISABLE_REGISTRATION=false`
to open it.

## Isolation and trust

- **No in-stack runner.** Workflow steps are arbitrary code. The old runner
  service gave that code the host's Docker socket, which is root-equivalent on the
  host (ADR 0027). The plugin no longer ships it, and mounts no Docker socket.
- **Register runners only on a CI node that is not a deploy host.** The ops
  profile usually lands on a staging or production host. Those hosts are not
  eligible to run CI jobs. Run `forgejo-runner` on a separate, disposable CI node
  and point it at this Forgejo's URL.
- **Not for fork pull requests.** Do not let a runner execute workflows from forks
  or from users you do not trust, and keep registration closed unless you need it.

The runner must reach the forge at its `ROOT_URL`. Forgejo publishes only on
`127.0.0.1:3844` and `ROOT_URL` is `http://<FORGEJO_DOMAIN>:3844/`, so put a TLS
reverse proxy or a tunnel in front, set `FORGEJO_DOMAIN`, and do not expose port 3844
to the internet as it is.

To register a runner on the CI node, create a token under Site administration or
the repository's Actions settings, then:

```bash
forgejo-runner register --no-interactive \
  --instance https://<your-forgejo-host>/ --token <registration-token> \
  --name ci-node-1 --labels ubuntu-latest:docker://node:20-bullseye
forgejo-runner daemon
```

## Migrating from the in-stack runner

Users of the removed `forgejo-runner` service, on plugin update:

```bash
nself build --remove-orphans
nself start
docker volume rm <project>_forgejo_runner_config
```

The first command removes the orphaned runner container. The second recreates
`forgejo` and `forgejo-init` from the new fragment, which releases the old runner
volume. The third deletes the stored runner state and registration token. If the
old runner network is still listed, `docker network rm <project>_forgejo_runner_net`
removes it. The Forgejo data volume and the admin user are untouched. Remove the `FORGEJO_RUNNER_*`
variables from your `.env`; they are no longer read. Registrations created on the
Forgejo server by the old runner stay in its database until you delete them in the
admin panel.

After the upgrade, audit what the open forge collected. The old init ran the admin
bootstrap as root, which Forgejo refuses, so on some installs the admin was never
created and the configured admin name could have been registered by anyone while
sign-up was open. The new init fails when that name exists without admin rights.
List the accounts in Site administration and remove any you did not create. Also
delete the old runner under Site administration > Actions > Runners and reset the
registration token there: a copy of the old token may sit in a `.env` file or backup.

Self-service sign-up is now closed by default. A deployment that relied on open
registration must set `FORGEJO_DISABLE_REGISTRATION=false` to keep it.

## License

MIT.
