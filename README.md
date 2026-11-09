# rules_helm

Bazel rules for downloading and templating Helm charts. Use the rendered YAML
directly, or transform it with tools such as `rules_jsonnet`. Publishing charts
and deploying to Kubernetes are outside this ruleset's scope.

**Requirements:** Bazel 9 or newer. The resolver uses prebuilt binaries for Linux
and macOS, on amd64 and arm64; consumers do not need Go installed. For an
unreleased checkout, see the [local resolver setup](cmd/helmlock/README.md#build-and-use-a-local-resolver).

## Quick start

Add `rules_helm` to your `MODULE.bazel` using your chosen release or source
override. See the [examples module](examples/MODULE.bazel) for a local checkout.

### 1. Declare chart dependencies

Keep dependencies in an ordinary `Chart.yaml`. The rules read it without
rewriting its comments, quotes, or formatting, so Renovate can update it normally.

```yaml
apiVersion: v2
name: thirdparty
version: 0.1.0
dependencies:
  - name: cert-manager
    version: "v1.12.0"
    repository: https://charts.jetstack.io
  - name: woodpecker
    version: "3.6.4"
    repository: oci://ghcr.io/woodpecker-ci/helm
```

Read the file from `MODULE.bazel`. You can give the generated repositories shorter
local names with `use_repo`:

```starlark
helm_deps = use_extension("@rules_helm//:extensions.bzl", "helm_deps")
helm_deps.from_file(chart = "//:Chart.yaml")
use_repo(
    helm_deps,
    cert_manager = "io_jetstack_charts_cert_manager",
    woodpecker = "io_ghcr_woodpecker_ci_helm_woodpecker",
)
```

### 2. Template a chart

Add a `values.yaml` with your chart configuration and create `BUILD.bazel`:

```starlark
load("@rules_helm//:helm.bzl", "helm_template")

exports_files(["Chart.yaml"])

helm_template(
    name = "cert_manager",
    chart = "@cert_manager//:chart",
    namespace = "cert-manager",
    values = ":values.yaml",
    out = "cert-manager.yaml",
)
```

```sh
bazel mod deps
bazel build //:cert_manager
```

The rendered manifests are written to `bazel-bin/cert-manager.yaml`.

## Updating dependencies

Change versions in `Chart.yaml`, then run:

```sh
bazel mod deps
```

Commit both `Chart.yaml` and `MODULE.bazel.lock`. Renovate's post-update automation
should run the same command. `bazel mod tidy` can update `use_repo` declarations
when repository names change.

Resolved URLs and chart archive digests are stored as facts in
`MODULE.bazel.lock`; there is no separate chart lockfile. Existing resolutions are
reused, and Bazel verifies downloaded archives against their pinned SHA-256.
Republishing an OCI version tag with different chart content fails verification.

## Versions and repository names

- Only exact versions are supported, not version ranges, repository aliases, or
  local `file://` dependencies.
- Requests for the same repository, chart, and version are deduplicated across
  modules. Different versions can coexist.
- Currently, a single version uses an unversioned repository name. When multiple
  versions of that chart/repository are requested, each name gets a version
  suffix, such as `io_jetstack_charts_cert_manager_v1_12_0`. See
  [the multi-version example](examples/Chart.yaml).
- Choose your own local names with `use_repo` renaming or BUILD `alias()` targets.
  Conflicting generated names are errors rather than silent overrides.

For explicit URLs and digests instead of a `Chart.yaml` input, use
`helm_deps.chart(...)`; see the [extension reference](docs/extensions.md).

## Private OCI registries

Resolution uses the Go container-registry default credential keychain. Chart
downloads support Docker `auths` files, but do not support all of the credential
helpers available during resolution. Authentication tokens are not stored in
Bazel facts. See the [repository rule reference](docs/rules.md#helm_repo_chart)
for download authentication attributes.

## Migrating from the chart lockfile API

Replace `helm_deps.lockfile(lockfile = ...)` with
`helm_deps.from_file(chart = ...)`. Remove `Chart.lock.json` and BUILD targets
using `helm_lock_file` or `generate_lockfile`; those APIs have been removed.
`helm_deps.chart(...)` remains available for explicitly pinned dependencies.

## Reference and contributing

- [Rules and toolchains](docs/rules.md)
- [Module extensions and their tags](docs/extensions.md)
- [Providers](docs/providers.md)
- [Working examples](examples/)
- [Contributing, testing, documentation generation, and releases](CONTRIBUTING.md)
