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

## Custom resource definitions

The chart renders its Custom Resource Definitions (CRDs) as regular templates, so `helm install` and
`helm upgrade` install and update the CRDs together with the operator. No separate CRD installation step
is required.

If you prefer to manage the CRD lifecycle yourself, you can split the installation into two steps:

```sh
# Step 1: Install only the CRDs
helm install shopware-crds shopware/operator --set crds.installOnly=true

# Step 2: Install the operator without CRDs
helm install operator shopware/operator --namespace my-namespace --create-namespace --set crds.install=false
```

# Configuration

## Metrics server

The operator can expose an HTTP endpoint with metrics about the stores it manages. It is disabled by
default and is enabled with `metrics.enabled=true`. Prometheus is not required for this. When enabled, the
chart creates a `shopware-operator` `Service` that exposes the endpoint on `metrics.port` (default `8080`),
and the operator injects `OPERATOR_SERVICE_URL` into every store container (admin, storefront and worker)
so the Shopware consumer can reach it. The URL defaults to
`http://shopware-operator.<namespace>.svc.cluster.local:<port>` and can be overridden with
`metrics.shopwareOperatorUrl`, for example when the operator is reachable under a different service name.

```sh
helm upgrade --install operator shopware/operator --namespace my-namespace --create-namespace \
  --set metrics.enabled=true
```

The endpoint serves two things:

* `/metrics` with the store metrics in the Prometheus text format, for example the store state, the
  scheduled task status and, when KEDA is enabled, `shopware_store_queue_count` per messenger transport.
* `/api/queue/<namespace>/<store>/<queue>` with the current length of a single queue as JSON, for example
  `{"store":"my-shop","namespace":"my-namespace","queue":"async","count":42}`. This route is only
  registered when `keda.enabled=true` and is what the `ScaledObject` polls, see
  [Worker autoscaling with KEDA](#worker-autoscaling-with-keda). The counts are read from the admin pod on
  demand and cached for 10 seconds.

Both routes can be used without any monitoring stack, which is useful to check the values by hand:

```sh
kubectl port-forward -n my-namespace svc/shopware-operator 8080:8080
curl http://localhost:8080/metrics
curl http://localhost:8080/api/queue/my-namespace/my-shop/async
```

If you do run Prometheus, the chart can additionally render a `ServiceMonitor` with
`metrics.serviceMonitor.enabled=true` so the endpoint is scraped automatically. This is optional and only
this part needs Prometheus. The chart does not ship the Prometheus Operator CRDs, so they have to be
installed beforehand.
Afterwards, enable the `ServiceMonitor` and match the label selector of your Prometheus instance:

```yaml
metrics:
  enabled: true
  port: 8080
  serviceMonitor:
    enabled: true
    interval: 30s
    scrapeTimeout: 10s
    additionalLabels:
      release: prometheus
```

> [!WARNING]
> Do not enable `metrics.serviceMonitor.enabled` without the Prometheus Operator CRDs being installed in
> the cluster. The `ServiceMonitor` resource cannot be created and the release will fail.

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

The operator can scale the Shopware message queue workers based on the queue length using
[KEDA](https://keda.sh/). This is optional and disabled by default. Without KEDA the operator creates a
single worker deployment that consumes all queues with a fixed replica count.

With `keda.enabled=true` the operator creates one worker deployment per queue and a KEDA `ScaledObject`
for each of them. The `ScaledObject` uses the `metrics-api` trigger and polls
`<operator metrics url>/api/queue/<namespace>/<store>/<queue>`, so the metrics endpoint is required, see
[Metrics server](#metrics-server). Additional transports configured in the shop are picked up automatically
once the operator has collected the queue statistics from the admin pod.

KEDA has to be enabled in two places: here, so the operator watches the KEDA resources, and on the store
itself with `spec.worker.enableKedaScaling`, so the operator creates the per queue deployments and
`ScaledObject` resources for it.

1. Install the KEDA operator together with its CRDs. This chart does not ship them.

   ```sh
   helm repo add kedacore https://kedacore.github.io/charts
   helm install keda kedacore/keda --namespace keda --create-namespace
   ```

2. Install or upgrade the operator with KEDA and the metrics endpoint enabled:

   ```sh
   helm upgrade --install operator shopware/operator --namespace my-namespace --create-namespace \
     --set keda.enabled=true \
     --set metrics.enabled=true
   ```

   The chart fails the render if `keda.enabled` is set without `metrics.enabled`, and the operator exits on
   startup if the KEDA CRDs are missing in the cluster.

3. Enable the scaling on the store, either with the
   [Shopware chart](https://github.com/shopware/helm-charts/tree/main/charts/shopware) through
   `store.worker.enableKedaScaling=true`, or directly on the `Store` resource:

   ```yaml
   spec:
     worker:
       enableKedaScaling: true
       # MaxReplicas which are spawned per queue. Minimum 1.
       maxReplicas: 3
       # MinReplicas which are spawned per queue. Minimum 0, so a queue can scale to zero.
       minReplicas: 0
       # Seconds keda waits after the last trigger before scaling down to minReplicas. Minimum 0.
       cooldownPeriod: 30
       # Seconds between keda queue length checks. Minimum 1.
       pollingInterval: 10
       # Queue length per worker replica keda scales towards. Minimum 1.
       targetQueueLength: 100
   ```

> [!NOTE]
> The queue statistics are collected from the admin pod, so the admin deployment must be running for the
> workers to scale.
