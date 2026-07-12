# Kubernetes test overlay

This overlay extends the shared Kubernetes base for a non-production test environment.

It provides:

- two replicas for the API Gateway, four business services and both Worker types;
- one `PodDisruptionBudget` per multi-replica Deployment with `minAvailable: 1`;
- one Gateway `Ingress` using `ingressClassName: nginx`;
- cluster-internal MySQL, RabbitMQ and business Services;
- placeholder Secret values that must be replaced by the deployment environment.

## Render

```bash
kustomize build deploy/kubernetes/overlays/test > /tmp/go-order-test.yaml
```

## Ingress prerequisite

The target cluster must already provide an ingress controller for the `nginx` ingress class.
The overlay does not install or manage an ingress controller.

The default host is:

```text
go-order.test.local
```

For a local test cluster, map the ingress address to that host in the local hosts file or use an equivalent DNS record.

Example request after DNS and the ingress controller are configured:

```bash
curl --fail http://go-order.test.local/readyz
```

## Secret boundary

The committed values are placeholders for rendering and isolated tests only. Do not use them as real environment credentials. A deployment workflow must supply the final Secret through its secret-management system.

## Deferred

This overlay intentionally does not include:

- TLS certificates;
- HPA;
- NetworkPolicy;
- managed-database configuration;
- production registry image tags;
- cloud-specific load balancer annotations.
