# Shopware Operator

![Shopware Kubernetes Operator](shopware.svg)

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shopware/shopware-operator)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/shopware/shopware-operator)
[![Go Report Card](https://goreportcard.com/badge/github.com/shopware/shopware-operator)](https://goreportcard.com/report/github.com/shopware/shopware-operator)

## Overview

This repository contains the Shopware Operator for Kubernetes. The Operator is a Kubernetes controller that manages Shopware installations in a Kubernetes cluster.

## Disclaimer

This Shopware operator is currently in an experimental phase and is not yet ready for production use.
The features, functionalities, and individual steps described in this repository are still under
development and are not in a final state. As such, they may contain bugs, incomplete
implementations, or other issues that could affect the stability and performance of your
Shopware installation.

Please be aware that using this operator in a live environment could lead to unexpected
behavior, data loss, or other critical problems. We strongly recommend using this operator
for testing and development purposes only.

By using this software, you acknowledge that you understand these risks and agree not
to hold the developers or maintainers of this repository liable for any
damage or loss that may occur.

If you encounter any issues or have suggestions for improvements, please feel free to
open an issue or contribute to the project.

## Installation

Below you find a descriptions how to deploy the Operator using `helm` or `kubectl`.

### Helm

For a helm installation check out our [charts repository](https://github.com/shopware/helm-charts/tree/main/charts/shopware-operator)

### kubectl

1. Install the custom resource definitions (cdr) for your cluster:

   ```sh
   kubectl apply -f https://github.com/shopware/shopware-operator/releases/latest/download/crd.yaml --server-side
   ```

2. Deploy the operator itself from `manager.yaml`:

   ```sh
   kubectl apply -f https://github.com/shopware/shopware-operator/releases/latest/download/manager.yaml
   ```

> [!IMPORTANT]
> This will install the Operator in the default namespace, if you want to change this use `kubectl -n <namespace> apply -f ...`

## Limitations and Issues

#### Sidecars

When using sidecars, please ensure they are properly terminated. Unfortunately, Kubernetes does not provide a reliable mechanism for
managing the shutdown of jobs (such as setup and migration jobs). As a result, we cannot guarantee that containers within the pod will
be stopped correctly. To address this, the job will be deleted once the operator container has completed its task.

## Contributing

Shopware welcomes community contributions to help improving the Shopware Operator.
If you found a bug or want to change something create an issue before fixing/changing it.

Another good place to discuss the Shopware Operator with developers and other community members is the Slack channel: <https://shopwarecommunity.slack.com/channels/shopware6-kubernetes>
