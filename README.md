# Shopware Operator

![Shopware Kubernetes Operator](shopware.svg)

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shopware/shopware-operator)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/shopware/shopware-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/shopware/shopware-operator)](https://goreportcard.com/report/github.com/shopware/shopware-operator)

## Overview

This repository contains the Shopware Operator for Kubernetes. The Operator is a Kubernetes controller that manages Shopware installations in a Kubernetes cluster.

## Installation

Below you find a descriptions how to deploy the Operator using `helm` or `kubectl`.

The validating webhook is enabled by default and requires [cert-manager](https://cert-manager.io/) to issue and rotate its certificate. Helm installations can disable it with `--set webhook.enabled=false`; this also removes the cert-manager dependency, but container overrides will no longer receive webhook validation.

For Helm installations, the webhook validates `Store` resources only in the
operator's release namespace by default. This allows multiple operator versions
to be installed in separate namespaces without their webhooks affecting one
another. A custom `webhook.namespaceSelector` can be configured when an operator
needs to manage additional namespaces. The default value `{}` means “use the
operator's release namespace”; to select all namespaces explicitly, configure
`namespaceSelector.matchExpressions: []`.

### Helm

For a helm installation check out our [charts repository](https://github.com/shopware/helm-charts/tree/main/charts/shopware-operator)

### kubectl

1. Install the custom resource definitions (cdr) for your cluster:

   ```sh
   kubectl apply -f https://github.com/shopware/shopware-operator/releases/latest/download/crd.yaml --server-side
   ```

2. Deploy the operator itself from `manager.yaml` into the `default` namespace:

   ```sh
   kubectl apply -f https://github.com/shopware/shopware-operator/releases/latest/download/manager.yaml
   ```

> [!IMPORTANT]
> The released `manager.yaml` wires its cluster-scoped webhook configuration to the `default` namespace. Use the Helm chart or a Kustomize overlay for installations in another namespace.

For a source-based Kustomize installation without the webhook or cert-manager resources, use:

```sh
kubectl apply -k config/no-webhook
```

## Local Development

To set up a local development environment, you must have the following components in place:

- A valid Store custom resource
- A MySQL-compatible database
- An S3-compatible object storage

These are required for a basic Shopware deployment within a Kubernetes cluster.

We recommend using the [Shopware Helm Chart](https://github.com/shopware/helm-charts/tree/main/charts/shopware), which includes a Percona-based MySQL database and an S3-compatible interface
provided by MinIO.To run the operator within your cluster, execute the following command:

```sh
NAMESPACE=default make run
```

The local controller runs without the validating webhook because cert-manager only provisions its TLS certificate for in-cluster deployments. Admission validation remains enabled by default for Helm and Kustomize installations that include the webhook resources.

> [!IMPORTANT]
> Ensure that you are using the correct Kubernetes context before running the command.

## Limitations and Issues

#### Sidecars

When using sidecars, please ensure they are properly terminated. Unfortunately, Kubernetes does not provide a reliable mechanism for
managing the shutdown of jobs (such as setup and migration jobs). As a result, we cannot guarantee that containers within the pod will
be stopped correctly. To address this, the job will be deleted once the operator container has completed its task.

## Contributing

Shopware welcomes community contributions to help improving the Shopware Operator.
If you found a bug or want to change something create an issue before fixing/changing it.

Another good place to discuss the Shopware Operator with developers and other community members is the Slack channel: <https://shopwarecommunity.slack.com/channels/shopware6-kubernetes>
