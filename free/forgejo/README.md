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

Self-service sign-up is now closed by default. A deployment that relied on open
registration must set `FORGEJO_DISABLE_REGISTRATION=false` to keep it.

## License

MIT.
