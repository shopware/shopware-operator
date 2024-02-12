# Shopware Operator

Useful links
* [Operator GitHub repository](https://github.com/shopware/shopware-operator)

## Pre-requisites
* Kubernetes 1.28+
* Helm v3

# Installation

This chart will deploy the Shopware Operator in you Kubernetes cluster.

## Installing the Chart
To install the chart using a dedicated namespace is recommended:

```sh
helm repo add shopware https://shopware.github.io/helm-charts/
helm install my-operator shopware/operator --version 0.1.0 --namespace my-namespace
```

Checkout the [values.yaml](values.yaml) file to modify the operator deployment.
Change it to your needs and install it:
```sh
helm install operator -f values.yaml shopware/operator
```
