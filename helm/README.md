# Shopware Operator

Useful links
* [Operator GitHub repository](https://github.com/shopware/shopware-operator)

## Pre-requisites
* Kubernetes 1.28+
* Helm v3

# Disclaimer

This Shopware Helm chart is currently in an experimental phase and is not ready for
production use. The services, configurations, and individual steps described in this
repository are still under active development and are not in a final state.
As such, they are subject to change at any time and may contain bugs,
incomplete implementations, or other issues that could affect the stability and performance
of your Shopware installation.

Please be aware that using this Helm chart in a live environment could lead to
unexpected behavior, data loss, or other critical problems. We strongly recommend using
this Helm chart for testing and development purposes only.

By using this software, you acknowledge that you understand these risks and agree not
to hold the developers or maintainers of this repository liable for any damage or
loss that may occur.

If you encounter any issues or have suggestions for improvements, please feel free to
open an issue or contribute to the project.

# Installation

This chart will deploy the Shopware Operator in you Kubernetes cluster.

## Installing the Chart
To install the chart using a dedicated namespace is recommended:

```sh
helm repo add shopware https://shopware.github.io/helm-charts/
helm install my-operator shopware/operator --namespace my-namespace --create-namespace
```

Checkout the [values.yaml](values.yaml) file to modify the operator deployment.
Change it to your needs and install it:
```sh
helm install operator shopware/operator -f values.yaml
```
