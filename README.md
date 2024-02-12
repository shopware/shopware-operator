# Shopware Operator

![Shopware Kubernetes Operator](shopware.svg)

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

<!-- ![Docker Pulls](https://img.shields.io/docker/pulls/percona/percona-xtradb-cluster-operator) -->
<!-- ![Docker Image Size (latest by date)](https://img.shields.io/docker/image-size/percona/percona-xtradb-cluster-operator) -->
<!-- ![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/percona/percona-xtradb-cluster-operator) -->
<!-- ![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/percona/percona-xtradb-cluster-operator) -->

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
    kubectl apply -f https://github.com/shopware/shopware-operator/releases/latest/download/crd.yaml
    ```

2. Deploy the operator itself from `manager.yaml`:

    ```sh
    kubectl apply -f https://github.com/shopware/shopware-operator/releases/latest/download/manager.yaml
    ```

> [!IMPORTANT]
> This will install the Operator in the default namespace, if you want to change this use `kubectl -n <namespace> apply -f ...`


## Contributing

Shopware welcomes community contributions to help improving the Shopware Operator.
If you found a bug or want to change something create an issue before fixing/changing it.

Another good place to discuss the Shopware Operator with developers and other community members is the Slack channel: https://shopwarecommunity.slack.com/channels/shopware6-kubernetes
