# deploy-pr

A small Go CLI that turns a GitHub pull request into an ephemeral preview environment on Amazon EKS. One command builds the PR's commit, pushes it to ECR, and rolls a Helm release into a per-PR namespace reachable at `https://pr-<N>.preview.<your-domain>`. The same binary works from a developer laptop and inside GitHub Actions.

## Quickstart

```bash
# Install
go install github.com/marcioaguiar/deploy-pr@latest

# Deploy
deploy-pr up 123 --comment-on-pr

# Tear down
deploy-pr down 123 --comment-on-pr

# See what's running
deploy-pr list
```

## Prerequisites

The CLI orchestrates external systems; you bring the systems.

| Tool | Why |
|---|---|
| Docker (with buildx) on PATH | local build of the PR's commit |
| AWS credentials (SSO, env, or IMDS) | ECR auth + EKS describe |
| A kubeconfig pointing at your EKS cluster | `aws eks update-kubeconfig --name <cluster>` |
| ECR repository | image push target |
| Ingress controller in the cluster (default: ingress-nginx) | routing |
| ExternalDNS + Route 53 (or equivalent) | resolving `pr-<N>.preview.<domain>` |
| Wildcard A/AAAA record `*.preview.<domain>` | DNS surface |
| `gh` CLI (optional) | fallback GitHub token source |

## Configuration

`deploy-pr` reads, in increasing precedence order:

1. Defaults baked into the binary
2. `.deploy-pr.yaml` in the current directory (or `--config <path>`)
3. Environment variables prefixed `DEPLOY_PR_` (nested keys: `DEPLOY_PR_NAMESPACE_PREFIX`)
4. CLI flags

Minimum config to run `up`:

```yaml
ecr_repo_uri: 000000000000.dkr.ecr.us-east-1.amazonaws.com/myapp
cluster_name: dev-cluster
base_domain: preview.example.com
```

A documented sample is checked into the repo as [`.deploy-pr.yaml`](.deploy-pr.yaml).

## Commands

```text
deploy-pr up <PR#> [--comment-on-pr] [--dry-run] [--image-tag <tag>] [--platform <os/arch>]
deploy-pr down <PR#> [--comment-on-pr] [--keep-namespace]
deploy-pr list [-o text|json]
deploy-pr version
```

Global flags: `--config`, `--repo owner/name`, `--github-token`, `--log-format text|json`, `-v / --verbose`.

`up` is idempotent: re-running on the same PR rolls the new commit's image without disturbing the namespace.

## GitHub Actions

A reference workflow lives at [`.github/workflows/preview.yml`](.github/workflows/preview.yml). It runs `up` on `pull_request` opened/synchronize/reopened and `down` on closed, authenticating to AWS via OIDC.

For the IAM setup the workflow depends on, see [`docs/aws-oidc-setup.md`](docs/aws-oidc-setup.md).

## How it works

```text
deploy-pr up 123
  -> resolve PR head SHA via GitHub API
  -> aws ecr get-authorization-token + docker login
  -> docker buildx build --tag <repo>:pr-123-<sha> --push
  -> helm upgrade --install pr-123 charts/preview
       --namespace pr-123 --create-namespace
       --set image.tag=pr-123-<sha>
       --set host=pr-123.preview.example.com
  -> post or update sticky PR comment with the URL
```

The Helm chart at [`charts/preview`](charts/preview) renders Namespace+Deployment+Service+Ingress with provenance labels (`deploy-pr/pr`, `deploy-pr/sha`, `app.kubernetes.io/managed-by=deploy-pr`) so `deploy-pr list` and any future reaper can find the cluster's preview footprint by selector.

## Troubleshooting

**`docker CLI not on PATH`** — install Docker Desktop or the docker engine.

**`ECR GetAuthorizationToken: AccessDenied`** — check the IAM role's trust policy and ensure the principal can reach ECR. From CI, this almost always means the OIDC role was misconfigured (see [`docs/aws-oidc-setup.md`](docs/aws-oidc-setup.md) Section 2).

**`helm upgrade --install: ... timeout waiting for ready state`** — `up` fails if pods don't reach Ready within `--timeout` (default 10m). Inspect with `kubectl -n pr-<N> describe pod`.

**`namespaces is forbidden: ... cannot create resource "namespaces" ... at the cluster scope`** — the principal lacks cluster-scoped `create namespaces` RBAC. Either grant it (see [`docs/aws-oidc-setup.md`](docs/aws-oidc-setup.md) §4 Path A) or pre-create `pr-<N>` and pass `--create-namespace=false` (Path B).

**`deploy-pr list` shows status `orphaned`** — namespace exists but no Helm release. The CI `down` job likely failed; clean up with `deploy-pr down <PR>` from a laptop.

**`exec format error` in pod logs** — image was built for the wrong CPU architecture. Default build target is `linux/amd64`; on Graviton/arm64 nodes set `platform: linux/arm64` in `.deploy-pr.yaml`, export `DEPLOY_PR_PLATFORM=linux/arm64`, or pass `--platform linux/arm64`. Note that the Dockerfile must also honor `$TARGETARCH` (e.g. `GOARCH=$TARGETARCH go build`) or the cross-arch build will still embed a host-native binary.

**Preview URL doesn't resolve** — confirm `*.preview.<domain>` has a wildcard DNS record, ExternalDNS is running, and the ingress class matches your controller.

## Development

```bash
git clone https://github.com/marcioaguiar/deploy-pr
cd deploy-pr
go build ./...
go test ./...
```

## License

MIT (TBD).
