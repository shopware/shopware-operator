# Shopware Operator

Useful links

* [Operator GitHub repository](https://github.com/shopware/shopware-operator)

## Pre-requisites

* Kubernetes 1.28+
* Helm v3
* cert-manager, if the validating webhook is enabled

# Installation

This chart will deploy the Shopware Operator in you Kubernetes cluster.

## Installing the Chart

To install the chart using a dedicated namespace is recommended:

```sh
helm repo add shopware https://shopware.github.io/helm-charts/
helm install my-operator shopware/operator --namespace my-namespace --create-namespace
```

To validate container overrides with the validating webhook, enable it. This requires cert-manager in the cluster:

```sh
helm install my-operator shopware/operator --namespace my-namespace --create-namespace --set webhook.enabled=true
```

Checkout the [values.yaml](values.yaml) file to modify the operator deployment.
Change it to your needs and install it:
```sh
helm install operator shopware/operator -f values.yaml
```


## Webhook

The `Store` resource contains schemaless container override fields, which the Kubernetes API server cannot
validate on its own. A mistake in those fields is therefore only noticed when the operator reconciles the
store. To catch this earlier, the chart can install a `ValidatingWebhookConfiguration` that validates every
`Store` on `CREATE` and `UPDATE` before it is persisted. The webhook is disabled by default.

The webhook needs a TLS certificate. The chart creates a self signed cert-manager `Issuer` and
`Certificate` for it and lets cert-manager inject the CA bundle into the webhook configuration, so
cert-manager has to be installed in the cluster. This chart does not ship it:

```sh
helm repo add jetstack https://charts.jetstack.io
helm install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace \
  --set crds.enabled=true
```

Then install or upgrade the operator with the webhook enabled:

```sh
helm upgrade --install operator shopware/operator --namespace my-namespace --create-namespace \
  --set webhook.enabled=true
```

By default the webhook only validates stores in the release namespace of the operator. Use
`webhook.namespaceSelector` to change this, for example to validate stores in all namespaces:

```yaml
webhook:
  enabled: true
  namespaceSelector:
    matchExpressions: []
```

After the installation you can verify that the webhook is serving:

```sh
kubectl get validatingwebhookconfigurations | grep shopware-operator
kubectl get certificate -n my-namespace
```

> [!WARNING]
> The webhook uses `failurePolicy: Fail`. If cert-manager is missing or the webhook pod is not reachable,
> every create and update of a `Store` is rejected. The operator also exits on startup if the webhook is
> enabled and the cert-manager CRDs are not installed.

## Worker autoscaling with KEDA
