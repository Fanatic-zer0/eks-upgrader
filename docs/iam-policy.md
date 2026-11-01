# IAM Policy

`eks-upgrader` itself only ever calls read-only AWS APIs; this policy is deliberately
much narrower than the credentials `eksctl` needs for mutation (CloudFormation,
EC2, IAM, EKS write APIs — see eksctl's own documentation for that broader policy).
Attach this policy to the role/IRSA/CI-OIDC identity used to run `eks-upgrader`
itself; `eksctl` should be invoked under its own, separately scoped broader role.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "EKSReadOnly",
      "Effect": "Allow",
      "Action": [
        "eks:DescribeCluster",
        "eks:ListClusters",
        "eks:DescribeNodegroup",
        "eks:ListNodegroups",
        "eks:DescribeAddon",
        "eks:ListAddons",
        "eks:DescribeAddonVersions",
        "eks:ListInsights",
        "eks:DescribeInsight"
      ],
      "Resource": "*"
    },
    {
      "Sid": "EC2ReadOnly",
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeSubnets",
        "ec2:DescribeSecurityGroups",
        "ec2:DescribeRouteTables",
        "ec2:DescribeInstances",
        "ec2:DescribeVpcs"
      ],
      "Resource": "*"
    },
    {
      "Sid": "IAMReadOnly",
      "Effect": "Allow",
      "Action": [
        "iam:GetRole",
        "iam:GetOpenIDConnectProvider",
        "iam:ListOpenIDConnectProviders"
      ],
      "Resource": "*"
    },
    {
      "Sid": "CloudWatchReadOnly",
      "Effect": "Allow",
      "Action": [
        "cloudwatch:GetMetricStatistics",
        "cloudwatch:GetMetricData",
        "cloudwatch:ListMetrics"
      ],
      "Resource": "*"
    },
    {
      "Sid": "DynamoDBLockBackend",
      "Effect": "Allow",
      "Action": [
        "dynamodb:PutItem",
        "dynamodb:GetItem",
        "dynamodb:DeleteItem"
      ],
      "Resource": "arn:aws:dynamodb:*:*:table/eks-upgrader-locks"
    }
  ]
}
```

The `DynamoDBLockBackend` statement is only required when
`spec.execution.lock.backend: dynamodb` is configured; omit it when using the default
local file-based lock.

## Boundary with eksctl's IAM requirements

`eksctl` requires a substantially broader policy (CloudFormation stack management,
EC2 instance/ASG/launch-template management, IAM role/OIDC-provider creation, EKS
control-plane and node-group write APIs). That policy is out of scope for this
document — see the official `eksctl` documentation for its minimum IAM requirements.
Running `eksctl` under a separate, more privileged identity than `eks-upgrader`
itself keeps the blast radius of a compromised `eks-upgrader` process limited to
read-only access.
