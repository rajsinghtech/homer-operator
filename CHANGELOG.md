# Changelog

All notable changes to Homer Operator are documented here.

## [1.2.1] - 2026-08-13

### Added

- Align Homer configuration handling with upstream Homer, including direct
  service/item fields, additional pages, arbitrary preserved fields, PWA
  assets, and richer message/theme configuration. ([#104])
- Add black-box Kubernetes E2E coverage for the documented Helm installation,
  including dashboard lifecycle, updates, cleanup, page assets, and Ingress
  discovery. ([#104], [#105])
- Watch referenced external Homer configuration and asset ConfigMaps so edits
  trigger reconciliation immediately. Same-namespace references are supported
  directly; cross-namespace asset references are mirrored into the Dashboard's
  namespace. ([#105])

### Fixed

- Deduplicate Dashboard reconcile requests when one ConfigMap is referenced by
  both external configuration and custom assets. ([#105])
- Deep-copy open-ended Homer values such as `message.refreshInterval` and
  preserve nil entries in `ArrayObjects`. ([#105])
- Remove the obsolete `kube-rbac-proxy` metrics sidecar and secure metrics
  directly through controller-runtime authentication and authorization. ([#92])

### Changed

- Preserve upstream Homer configuration forms and explicit empty, null, and
  falsy values through CRD and YAML/JSON round trips. ([#104])
- Add service and item ordering through Homer annotations. ([#84])
- Refresh Kubernetes, Go, and related build dependencies carried forward from
  the `1.2.0` release.

### Verification

- `go test ./...` — passed, including 6/6 live E2E specifications.
- `go vet ./...` — passed.
- `make manifests`, `make build`, and `make build-installer` — passed.
- Helm lint/template, strict kubeconform, CRD dry-run, and `git diff --check`
  — passed.

[#84]: https://github.com/rajsinghtech/homer-operator/pull/84
[#92]: https://github.com/rajsinghtech/homer-operator/pull/92
[#104]: https://github.com/rajsinghtech/homer-operator/pull/104
[#105]: https://github.com/rajsinghtech/homer-operator/pull/105

[1.2.1]: https://github.com/rajsinghtech/homer-operator/compare/v1.2.0...v1.2.1
