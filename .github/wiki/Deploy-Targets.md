# Deploy targets

nSelf runs a project behind one reverse proxy on a Docker host. Three plugins widen where and how it runs: Traefik as the
proxy, Kubernetes through Helm, and OpenTofu or Terraform for servers. This page says which command does what and what the
recorded tests cover. Each plugin home carries its own maturity and version; read it there, not here.

| Target | Command surface | Plugin home |
|---|---|---|
| nginx (default) | `nself build`, `nself start` | the `nself` CLI; no plugin |
| Traefik | `nself add traefik`, `nself remove traefik` | [[Traefik\|plugins/Traefik]] |
| Kubernetes | `nself k8s values`, `install`, `upgrade`, `status`, `uninstall`, `backup`, `restore` | [[K8s\|plugins/K8s]] |
| Servers (OpenTofu or Terraform) | `nself infra validate`, `nself infra plan` | [[Infra\|plugins/Infra]] |

## nginx, the default

A new project uses nginx. `nself build` writes the provider-neutral route model to `.nself/generated/routes.json`; the
compose generator turns it into the nginx service. You do nothing to get this setup.

```bash
nself build
nself start
```

## Traefik instead of nginx

`nself add traefik` makes Traefik the proxy. The nginx service is omitted from the generated compose file and three Traefik
services take its place. Routes, TLS and upstreams come from the same route model, so a request returns the same status,
headers and body through either proxy (the parity test compares both in Linux containers).

```bash
nself add traefik
nself build --remove-orphans --yes
nself start
```

Go back to nginx:

```bash
nself remove traefik --yes
nself build --remove-orphans --yes
nself start
```

What the tests cover:

- The swap and the round trip run end to end on Linux. After `nself remove traefik` the container list before and after the
  swap is identical, so no Traefik container is left behind.
- Removal deletes only generated Traefik files that carry the `.nself-generated` marker, and never follows a symlink.
- An interrupted removal is finished by running `nself build --remove-orphans`, then `nself start`.
- Certificate renewal under Traefik is tested on Linux against a local ACME test server (pebble): the HTTP-01 challenge is
  answered, the served certificate changes, Traefik is not restarted, and a failed render rolls back to the previous
  certificate.

Limits, as recorded:

- Swapping the proxy restarts the stack. The measured swap downtime was 35 seconds on a hosted Linux runner.
- While a running Traefik is being removed, it serves no routes between the file cleanup and the container removal.
- `nself add traefik` refuses (before any write) when a project is fronted by another proxy or uses routes the Traefik
  renderer cannot express. Read the refusal; the plugin home lists the cases.
- Rootless Docker is not covered by the tests, and `nself ssl add --acme` under Traefik is not covered yet.
- The long-upload and slow-read behaviour differs from nginx in documented ways; see the upload entrypoint on the plugin home.

## Kubernetes on a cluster

`nself k8s` installs nSelf from a Helm chart that ships inside the plugin binary. The chart values are generated from the
same compose model that `nself start` uses, so the cluster runs what your project defines. You need `helm` and `kubectl`
on `PATH` and a reachable cluster.

```bash
nself build
nself k8s values
nself k8s install --domain myapp.com
nself k8s status
nself k8s upgrade --domain myapp.com
nself k8s uninstall --yes
```

Backup and restore of the Postgres database go through the core backup layout, with a content fingerprint checked on restore:

```bash
nself k8s backup
nself k8s restore <file> --yes
```

What the tests cover:

- Install and upgrade run on a kind cluster. Every pod becomes Ready and the GraphQL endpoint answers `__typename`.
- Backup and restore run on a kind cluster. Three tables with 1,000 rows each are seeded, the namespace is deleted and
  installed again, the dump is restored, and an independent digest of the tables matches.
- `nself k8s uninstall` without `--yes` and without a terminal exits 4 and leaves the release in place.

Limits, as recorded:

- The tests ran on kind only. Other cluster flavours (managed or bare-metal) have no recorded test run.
- The licence key is not written into the release yet; installs that set `NSELF_PLUGIN_LICENSE_KEY` print a warning.
- A restore needs a dump made by `nself k8s backup`. A dump from `nself backup` has no fingerprint sidecar and is refused.

## Servers with OpenTofu or Terraform

`nself infra` provisions Hetzner Cloud servers with OpenTofu (preferred) or Terraform. Check a module and read the plan;
neither command creates anything:

```bash
nself infra validate
nself infra plan
```

Limits, as recorded:

- The tests use mocked providers. Continuous integration never runs `apply` or `destroy` against a real cloud.
- `apply` and `destroy` cost money or remove servers. They need `--i-accept-cloud-spend` and an interactive terminal.
- Hetzner is the only shipped provider. The plugin home states its maturity and the other limits.
