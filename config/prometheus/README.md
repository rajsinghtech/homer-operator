# Optional Prometheus integration

The default static installation deliberately does not contain a
`monitoring.coreos.com/v1` `ServiceMonitor`. This keeps
`kubectl apply -k config/default` usable on clusters that do not install the
Prometheus Operator CRD.

After installing the Prometheus Operator, apply this optional
operator-plus-monitor overlay:

```bash
kubectl apply -k config/prometheus
```

The overlay renders the normal operator installation, its metrics `Service`,
the `/metrics` reader `ClusterRole`, and the `ServiceMonitor`. It is a
complete operator-plus-monitor render and is safe to apply after
`config/default`; applying the latter first is not required. It is not a
self-contained scrape authorization setup: the overlay cannot know which
Prometheus `ServiceAccount` your installation uses, so the
`ClusterRoleBinding` below is intentionally a manual step.

The rendered metrics resources are named
`homer-operator-controller-manager-metrics` (Service),
`homer-operator-controller-manager-metrics-monitor` (ServiceMonitor), and
`homer-operator-metrics-reader` (ClusterRole), all with the expected
operator namespace where applicable. The monitor targets the Service's
`https` port.

The operator serves authenticated HTTPS metrics. The `ServiceMonitor` uses
the Prometheus pod's bearer token, so bind the generated metrics reader role to
the Prometheus service account used by your Prometheus instance:

```bash
kubectl create clusterrolebinding homer-operator-prometheus-metrics \
  --clusterrole=homer-operator-metrics-reader \
  --serviceaccount=<prometheus-namespace>:<prometheus-service-account>
```

The monitor skips certificate verification because the controller-runtime
metrics server uses its in-cluster serving certificate. Restrict the role
binding to the Prometheus service account rather than granting it broadly.
