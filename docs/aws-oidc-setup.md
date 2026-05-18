# AWS OIDC setup for `deploy-pr` in GitHub Actions

This is a one-time setup so GitHub Actions can assume an IAM role without storing long-lived AWS credentials in repository secrets. The CI workflow at [`.github/workflows/preview.yml`](../.github/workflows/preview.yml) depends on it.

> **Conceptual companions (HTML, optional):** [`aws-oidc-setup-eli5.html`](./aws-oidc-setup-eli5.html) walks through this setup with diagrams; [`kubernetes-namespaces-eli5.html`](./kubernetes-namespaces-eli5.html) explains why namespace creation is structurally privileged; [`namespace-creation-risk-eli5.html`](./namespace-creation-risk-eli5.html) covers the threat models that make §4 of this doc matter.

## 1. Create the GitHub OIDC provider in IAM

If your AWS account has not yet trusted GitHub's OIDC issuer, create the provider once per account:

```bash
aws iam create-open-id-connect-provider \
  --url https://token.actions.githubusercontent.com \
  --client-id-list sts.amazonaws.com \
  --thumbprint-list 6938fd4d98bab03faadb97b34396831e3780aea1
```

The thumbprint is GitHub's; AWS now also auto-validates the chain so the value is informational. If your account already has this provider (check with `aws iam list-open-id-connect-providers`), skip this step.

## 2. Create the IAM role

The role is what GitHub Actions assumes. Two policies attach to it: a **trust policy** that says *who* may assume it, and a **permission policy** that says *what* the assumed credentials may do.

### Trust policy (`trust-policy.json`)

> **Critical:** scope `sub` to a specific repo and event. A wildcard like `repo:owner/*:*` would let any workflow in any repo assume this role -- including malicious forks.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::000000000000:oidc-provider/token.actions.githubusercontent.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
        },
        "StringLike": {
          "token.actions.githubusercontent.com:sub": "repo:OWNER/REPO:pull_request"
        }
      }
    }
  ]
}
```

Replace `000000000000` with your AWS account ID and `OWNER/REPO` with the GitHub repo where the workflow lives.

### Permission policy (`deploy-pr-policy.json`)

Least privilege: ECR push on the configured repository, EKS describe on the configured cluster, STS get-caller-identity for sanity. No `iam:*`, no `kms:*`, no broad `eks:*`.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ECRAuth",
      "Effect": "Allow",
      "Action": [
        "ecr:GetAuthorizationToken"
      ],
      "Resource": "*"
    },
    {
      "Sid": "ECRPushPullSpecificRepo",
      "Effect": "Allow",
      "Action": [
        "ecr:BatchCheckLayerAvailability",
        "ecr:CompleteLayerUpload",
        "ecr:InitiateLayerUpload",
        "ecr:PutImage",
        "ecr:UploadLayerPart",
        "ecr:BatchGetImage",
        "ecr:GetDownloadUrlForLayer"
      ],
      "Resource": "arn:aws:ecr:us-east-1:000000000000:repository/myapp"
    },
    {
      "Sid": "EKSDescribe",
      "Effect": "Allow",
      "Action": [
        "eks:DescribeCluster"
      ],
      "Resource": "arn:aws:eks:us-east-1:000000000000:cluster/dev-cluster"
    }
  ]
}
```

### Create the role and attach policies

```bash
aws iam create-role \
  --role-name github-deploy-pr \
  --assume-role-policy-document file://trust-policy.json

aws iam put-role-policy \
  --role-name github-deploy-pr \
  --policy-name deploy-pr-permissions \
  --policy-document file://deploy-pr-policy.json
```

The role ARN is the value you put in `AWS_ROLE_ARN` in `preview.yml`.

## 3. EKS access entry for the role

Granting `eks:DescribeCluster` lets the role read the cluster endpoint and CA, but doesn't grant Kubernetes-level access. Map the role to a Kubernetes group:

```bash
aws eks create-access-entry \
  --cluster-name dev-cluster \
  --principal-arn arn:aws:iam::000000000000:role/github-deploy-pr \
  --type STANDARD

aws eks associate-access-policy \
  --cluster-name dev-cluster \
  --principal-arn arn:aws:iam::000000000000:role/github-deploy-pr \
  --policy-arn arn:aws:eks::aws:cluster-access-policy/AmazonEKSEditPolicy \
  --access-scope type=cluster
```

`AmazonEKSEditPolicy` is broader than required (it grants edit across the whole cluster). For real production use, scope to a namespace pattern with `--access-scope type=namespace,namespaces=pr-*`. Note that a namespace-scoped principal cannot create the `pr-<N>` namespace itself -- see [§4](#4-kubernetes-rbac-for-namespace-creation) for the two ways to handle that.

## 4. Kubernetes RBAC for namespace creation

> **Want the conceptual "why" before the procedural "how"?** See [`kubernetes-namespaces-eli5.html`](./kubernetes-namespaces-eli5.html) for what a namespace actually is and why creating one is structurally cluster-scoped (and therefore can't be sandboxed to `pr-*` by RBAC alone).

`deploy-pr up` defaults to letting Helm create the target namespace (`--create-namespace`). Namespaces are cluster-scoped resources, so the principal needs Kubernetes RBAC that permits `create namespaces` at the cluster scope. A principal scoped via `--access-scope type=namespace,namespaces=pr-*` (the production guidance in §3) intentionally lacks that verb and will fail with:

```
namespaces is forbidden: User "..." cannot create resource "namespaces" in API group "" at the cluster scope
```

The same applies to a local IAM user (e.g. `arn:aws:iam::ACCOUNT:user/pr-deploy`) running `deploy-pr up` from a laptop -- not just the GitHub Actions role.

Pick one of the two paths below.

### Path A -- grant cluster-scoped namespace verbs (simpler default)

Map the principal to a Kubernetes group with `--kubernetes-groups`, then bind a ClusterRole to that group. This is on top of the access-entry from §3; you can either replace the broad `AmazonEKSEditPolicy` association with this group-based mapping, or layer it alongside a narrower namespace-scoped policy.

```bash
# GitHub Actions role
aws eks create-access-entry \
  --cluster-name dev-cluster \
  --principal-arn arn:aws:iam::000000000000:role/github-deploy-pr \
  --kubernetes-groups deploy-pr \
  --type STANDARD

# Local IAM user (e.g. your laptop user)
aws eks create-access-entry \
  --cluster-name dev-cluster \
  --principal-arn arn:aws:iam::000000000000:user/pr-deploy \
  --kubernetes-groups deploy-pr \
  --type STANDARD
```

Then apply the ClusterRole and binding (run as a cluster admin):

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: deploy-pr-namespace-manager
rules:
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["create", "get", "list", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: deploy-pr-namespace-manager
subjects:
  - kind: Group
    name: deploy-pr             # must match --kubernetes-groups above
    apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: deploy-pr-namespace-manager
  apiGroup: rbac.authorization.k8s.io
```

```bash
kubectl apply -f deploy-pr-rbac.yaml
```

> **Caveat:** Kubernetes RBAC cannot restrict `create` by resource name, so this grants creation of *any* namespace, not just `pr-*`. If that matters, layer a `ValidatingAdmissionPolicy` (or Kyverno/OPA) that rejects namespace creates from this group whose name doesn't match `pr-*`.

This group also needs verbs on the workload resources the chart installs (Deployment, Service, Ingress, Secret, etc.). The simplest layering is to keep the namespace-scoped `AmazonEKSEditPolicy` association from §3 *and* add this cluster-scoped binding for just the namespace verbs.

### Path B -- pre-create namespaces, run with `--create-namespace=false`

For environments where the principal must stay strictly namespace-scoped (no cluster verbs at all), an admin (or a controller) creates `pr-<N>` ahead of time:

```bash
kubectl create namespace pr-123
# optionally label it so deploy-pr list classifies it correctly
kubectl label namespace pr-123 app.kubernetes.io/managed-by=deploy-pr
```

Then invoke `up` with namespace creation disabled:

```bash
deploy-pr up 123 --create-namespace=false
```

The Helm install skips the namespace-create call, and the namespace-scoped principal only needs verbs inside the existing namespace.

## 5. Verify

In a draft PR, check the Actions tab for the **PR preview** workflow. The first run will fail in interesting places: missing wildcard DNS, ingress class mismatch, etc. The CLI's logs name the failing system; fix forward.

## Operational notes

- **ECR storage cost.** Set an ECR lifecycle policy to expire untagged images after 30 days and keep the most recent N tags per PR. The CLI does not do this; AWS lifecycle policies do.
- **Orphan namespaces.** If the `closed` workflow fails to run (e.g., the runner queue was at capacity), `deploy-pr list` will show the namespace with status `orphaned`. Run `deploy-pr down <PR>` from a laptop to clean up.
- **Drift between laptop and CI.** The same binary serves both, but a developer running `deploy-pr up <PR>` from their laptop may overwrite a CI-deployed release. That's fine for ephemeral previews; if it isn't, scope IAM so laptop credentials lack push privileges.
