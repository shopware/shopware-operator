# Agent Guidelines

## Cloud agnosticism and digital sovereignty

The Shopware Operator must run on any conformant Kubernetes cluster, regardless of
where it is hosted: public cloud (AWS, GCP, Azure), European or sovereign clouds
(STACKIT, IONOS, OVHcloud, Hetzner, Open Telekom Cloud, ...), or on-premises.
Never introduce a hard dependency on a single provider.

### Rules

- Depend only on Kubernetes APIs and open standards. Do not use provider-specific
  CRDs, annotations, controllers, or managed services as a requirement
  (e.g. AWS Load Balancer Controller annotations, GKE `BackendConfig`, Azure
  Workload Identity, EKS IRSA, Secrets Manager / Key Vault / Cloud KMS).
- Do not import provider SDKs (`cloud.google.com/go/...`, `github.com/Azure/...`,
  `github.com/aws/aws-sdk-go-v2/service/<anything other than s3>`, ...).
- Networking uses standard `networking.k8s.io` Ingress or Gateway API
  `HTTPRoute`. Ingress class, Gateway, and StorageClass names come from the CR or
  configuration, never hardcoded.
- Credentials are read from Kubernetes Secrets referenced in the CR. No reliance
  on instance metadata services, cloud IAM roles, or provider credential chains
  as the only way to authenticate.
- Endpoints, regions, and hostnames are configurable. Never hardcode provider
  URLs or default to a provider-specific endpoint without allowing an override.
- Databases, caches, OpenSearch, and similar services are consumed via standard
  protocols (MySQL, Redis, HTTP) and connection settings from the CR or Secrets,
  not via provider management APIs.
- Observability uses open standards: OpenTelemetry for tracing, Prometheus for
  metrics, structured logs to stdout.
- Container images, Helm charts, and manifests must not assume a specific
  registry or cloud. Keep image references configurable.
- Tests and examples must not require a specific cloud account. Prefer kind,
  MinIO, and other self-hostable components.

### Allowed exceptions

- **Object storage via the AWS S3 SDK.** `github.com/aws/aws-sdk-go-v2/service/s3`
  (with `aws`, `config`, `credentials`, and transport/retry helpers) is allowed
  because the S3 API is the de facto standard and works with every S3-compatible
  provider (MinIO, Ceph, STACKIT, IONOS, OVHcloud, Hetzner, Cloudflare R2, GCS
  interop, ...). Keep it provider-neutral:
  - Endpoint, region, and path-style addressing must be configurable.
  - Do not use AWS-only features (S3 Express, Object Lambda, Transfer
    Acceleration, AWS-specific SSE-KMS key handling, STS-only auth) without a
    portable fallback.
  - Use static access key / secret key credentials from Secrets as the
    supported baseline.
- **Events.** Event sinks are abstracted behind the `EventHandler` interface in
  `internal/event/event.go`. Implementations of that interface (e.g.
  `internal/event/nats`) may be provider-specific. Add new providers as separate
  implementations of the interface; never call a provider-specific event client
  from controllers or other packages directly.

### New dependencies and major changes require team approval

Adding a new third-party service or client (e.g. NATS, Kafka, a new database
driver, a vendor SDK) or making a major architectural change must not be done
on an agent's own initiative. Before writing such code:

1. Stop and tell the user that the change needs a GitHub issue and a team
   discussion.
2. Offer to draft the issue. It should cover the motivation, the proposed
   dependency or design, the alternatives considered, the impact on cloud
   agnosticism, and the maintenance and security cost.
3. Only implement it once the user confirms the team has agreed and links the
   issue.

This applies to new Go modules in `go.mod` that pull in an external service
client, new runtime components the operator talks to, and new CRDs or changes
to existing CRD semantics. Small utility libraries, version bumps, and changes
inside existing integrations do not need this process.

### When in doubt

If a feature seems to require a provider-specific integration, introduce a small
interface with a generic default implementation (as done for events) and put the
provider-specific code behind it. Flag the trade-off to the user instead of
silently adding the dependency.
