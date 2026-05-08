# How `deploy-pr` works

When someone opens a pull request, a reviewer usually has to either trust the screenshots or pull the branch and run it locally. `deploy-pr` removes that step: every PR gets its own live URL — `https://pr-123.preview.example.com` — running the exact code from that PR, hosted on a real Kubernetes cluster. When the PR closes, the environment disappears.

The tool itself is a single Go binary. It doesn't host anything. It glues together five things that already exist:

1. **GitHub** — to look up which commit a PR points at, and to comment back on the PR with the preview URL.
2. **Docker** — to build a container image from that commit.
3. **ECR** (Elastic Container Registry, AWS's image storage) — to host that image so the cluster can pull it.
4. **EKS** (Elastic Kubernetes Service, AWS's managed Kubernetes) — to actually run the image.
5. **Helm** — a templating tool that turns a small chart into the Kubernetes objects (Namespace, Deployment, Service, Ingress) needed to run and expose your app.

The flow for `deploy-pr up 123` is literally:

```
PR# -> GitHub API -> SHA
SHA -> docker build -> ECR push
ECR image -> helm upgrade --install -> EKS namespace pr-123
URL       -> sticky comment on PR
```

`deploy-pr down 123` reverses it: `helm uninstall` and delete the namespace.

## Where EKS fits

Kubernetes is the orchestrator that decides "this container runs here, on this node, exposed via this URL, with this much CPU." EKS is just **Kubernetes-as-a-service from AWS** — Amazon runs the control plane (the brain), you bring the workers (or let AWS run those too via Fargate). For this tool, EKS is the thing that:

- **Pulls the image from ECR** and runs it as a Pod.
- **Isolates each PR** via a Namespace (`pr-123`, `pr-456`, …) so two previews can't collide.
- **Routes traffic** via an Ingress controller (typically `ingress-nginx` running inside the cluster), which terminates HTTP(S) and forwards `pr-123.preview.example.com` to the right Service.
- **Gets a DNS name** via ExternalDNS, an in-cluster agent that watches Ingress objects and writes matching records into Route 53. That's what makes `pr-123.preview.example.com` actually resolve.

The Helm chart in [`charts/preview`](../charts/preview) is the recipe Helm uses to generate those Kubernetes objects. Each `helm upgrade --install pr-123 …` either creates them or rolls a new image into existing ones — which is why re-running `up` on the same PR is safe and idempotent.

## What you'd have to set up to use it on your repo

This is the part that surprises people new to it: the CLI is small, the surrounding infrastructure is the real cost. You need, **once**:

- An **EKS cluster** (`dev-cluster` in the examples).
- An **ECR repository** to push images to.
- **ingress-nginx** installed in the cluster.
- **ExternalDNS** installed and pointing at a Route 53 hosted zone.
- A **wildcard DNS record** `*.preview.your-domain.com` that points at the ingress controller's load balancer.
- A **TLS strategy** (cert-manager + Let's Encrypt is the usual choice) if you want HTTPS.
- An **IAM role** that GitHub Actions can assume via OIDC (see [`aws-oidc-setup.md`](aws-oidc-setup.md) — the role can push to ECR and talk to EKS, nothing else).

Then in the repo whose app you want previewed:

- Add `.github/workflows/preview.yml` (copy from this repo, change the `env:` block — your AWS account ID, your cluster, your domain, your ECR repo).
- Add a `Dockerfile` at the repo root (the build step assumes one exists).
- Add a `.deploy-pr.yaml` if you want non-default config.

## Triggering on a specific PR comment instead of every push

The reference workflow runs on every PR open/sync/close — every push to a PR rebuilds the preview. That's expensive if your build is slow. A common pattern is to gate it on a comment like `/preview` or `/deploy`.

GitHub fires the `issue_comment` event for PR comments (PRs are issues under the hood). You'd swap the trigger and add a guard:

```yaml
on:
  issue_comment:
    types: [created]
  pull_request:
    types: [closed]   # keep this so teardown still happens automatically

concurrency:
  group: preview-${{ github.event.issue.number || github.event.number }}
  cancel-in-progress: true

jobs:
  preview:
    # Only run on /preview comments on PRs, or on PR close.
    if: |
      (github.event_name == 'issue_comment'
        && github.event.issue.pull_request != null
        && startsWith(github.event.comment.body, '/preview'))
      || (github.event_name == 'pull_request' && github.event.action == 'closed')
    runs-on: ubuntu-latest
    steps:
      - name: Resolve PR number and ref
        id: pr
        run: |
          if [ "${{ github.event_name }}" = "issue_comment" ]; then
            echo "number=${{ github.event.issue.number }}" >> "$GITHUB_OUTPUT"
          else
            echo "number=${{ github.event.number }}" >> "$GITHUB_OUTPUT"
          fi

      # When triggered by a comment, github.sha is the default branch's HEAD,
      # not the PR's. Check out the PR explicitly.
      - uses: actions/checkout@v4
        with:
          ref: refs/pull/${{ steps.pr.outputs.number }}/head

      # ... rest of the steps (setup-go, configure-aws-credentials, etc.) ...

      - name: Deploy preview
        if: github.event_name == 'issue_comment'
        run: deploy-pr up ${{ steps.pr.outputs.number }} --comment-on-pr
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      - name: Tear down preview
        if: github.event.action == 'closed'
        run: deploy-pr down ${{ steps.pr.outputs.number }} --comment-on-pr
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

Things to know about this trigger:

1. **`issue_comment` fires on issues and PRs.** The `github.event.issue.pull_request != null` check filters out plain issues.
2. **PR number is at a different path.** `github.event.issue.number` for comments vs. `github.event.number` for `pull_request` events. The `id: pr` step normalizes that.
3. **Checkout defaults are wrong for comments.** On `issue_comment`, `github.sha` is the default branch, not the PR. `ref: refs/pull/<N>/head` fixes it.
4. **Permissions matter for security.** Anyone who can comment on a PR can now trigger an AWS-credentialed build. Tighten the trust policy in [`aws-oidc-setup.md`](aws-oidc-setup.md) and/or add a check like `github.event.comment.author_association == 'MEMBER'` (or `OWNER` / `COLLABORATOR`) to refuse comments from outside your team.

You could extend this further — `/preview down` to tear down without closing the PR, `/preview rebuild` to force a fresh build, etc. — by parsing `github.event.comment.body` and branching on the subcommand. The CLI itself doesn't need to change; only the workflow that calls it does.
