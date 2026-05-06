# AWS OIDC setup for `deploy-pr` in GitHub Actions

This is a one-time setup so GitHub Actions can assume an IAM role without storing long-lived AWS credentials in repository secrets. The CI workflow at [`.github/workflows/preview.yml`](../.github/workflows/preview.yml) depends on it.

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

`AmazonEKSEditPolicy` is broader than required (it grants edit across the whole cluster). For real production use, scope to a namespace pattern with `--access-scope type=namespace,namespaces=pr-*`.

## 4. Verify

In a draft PR, check the Actions tab for the **PR preview** workflow. The first run will fail in interesting places: missing wildcard DNS, ingress class mismatch, etc. The CLI's logs name the failing system; fix forward.

## Operational notes

- **ECR storage cost.** Set an ECR lifecycle policy to expire untagged images after 30 days and keep the most recent N tags per PR. The CLI does not do this; AWS lifecycle policies do.
- **Orphan namespaces.** If the `closed` workflow fails to run (e.g., the runner queue was at capacity), `deploy-pr list` will show the namespace with status `orphaned`. Run `deploy-pr down <PR>` from a laptop to clean up.
- **Drift between laptop and CI.** The same binary serves both, but a developer running `deploy-pr up <PR>` from their laptop may overwrite a CI-deployed release. That's fine for ephemeral previews; if it isn't, scope IAM so laptop credentials lack push privileges.
