# Shopware Operator

Useful links
* [Operator GitHub repository](https://github.com/shopware/shopware-operator)

## Pre-requisites
* Kubernetes 1.28+
* Helm v3
* cert-manager, unless the validating webhook is disabled

# Installation

This chart will deploy the Shopware Operator in you Kubernetes cluster.

## Installing the Chart
To install the chart using a dedicated namespace is recommended:

```sh
helm repo add shopware https://shopware.github.io/helm-charts/
helm install my-operator shopware/operator --namespace my-namespace --create-namespace
```

To install without cert-manager, disable container-override validation:

```sh
helm install my-operator shopware/operator --namespace my-namespace --create-namespace --set webhook.enabled=false
```

Checkout the [values.yaml](values.yaml) file to modify the operator deployment.
Change it to your needs and install it:
```sh
helm install operator shopware/operator -f values.yaml
```
