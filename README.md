# `helm_rules`

This set of rules are made with using `rules_jsonnet` in mind.

Mostly so jsonnet can be used to make modications to a templated helm chart.

Deploying helm charts to repositories or to a kubernetes cluster is not the goal of these rules.

## Rules

 * `helm_chart` - Basic wrapper around files to become a "Helm Chart"
 * `helm_repo_chart` - Pulls a chart from HTTP URLs or OCI repositories and validates it with sha256
 * `helm_template` - Templates the chart with a given set of `values.yaml`

## Todo

 - [x] Toolchains
    - [ ] Even more of them (arm, Windows, etc.)
    - [ ] Generate the version matrix from Helm's release page
 - [x] Pull external helm charts
      - [x] Using a repositorie's `index.yaml`
      - [x] Using OCI repositories via `helm pull`
 - [ ] Docs
 - [ ] Tests
 - [x] Working examples
 - [ ] Clean up code

## Examples

### Bzlmod chart dependencies (Bazel 9+)

Keep dependencies in an ordinary `Chart.yaml`. The extension reads the file without
rewriting its comments, quoting, or formatting, so Renovate's Helm manager can
continue updating it.

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

```starlark
# MODULE.bazel
helm_deps = use_extension("@rules_helm//:extensions.bzl", "helm_deps")
helm_deps.from_file(chart = "//:Chart.yaml")
use_repo(
    helm_deps,
    "io_jetstack_charts_cert_manager",
    "io_ghcr_woodpecker_ci_helm_woodpecker",
)
```

Export the input file from its `BUILD.bazel`:

```starlark
exports_files(["Chart.yaml"])
```

Run `bazel mod deps` after changing dependencies, then commit `Chart.yaml` and
`MODULE.bazel.lock`. `bazel mod tidy` can update the `use_repo` declarations.
Renovate automation should run `bazel mod deps` after its Helm updates.

Only exact versions are supported; version ranges, repository aliases, and local
`file://` dependencies are not supported. When multiple versions of the same
chart/repository are requested across the module graph, generated repository names
include a version suffix, as in the examples module. Conflicting generated names
are errors, not silently overridden dependencies.

The extension records one versioned fact per `(repository, chart, version)` and
returns `reproducible = True`. HTTP facts contain archive URLs and SHA-256; OCI
facts contain `repository`, `chart`, `version`, and the Helm content-layer archive
`digest`. Versions stay in `Chart.yaml`; digests stay in Bazel's lockfile. Existing
facts are reused without querying repository indexes or registry manifests.
The parser executable may still be downloaded if it is not cached, and chart
downloads still require access to their hosting repositories.

All chart archives are SHA-256 verified by Bazel. OCI archives are fetched through
the version tag and checked against the pinned archive digest: republishing the
tag with different content fails verification. Private OCI resolution uses the
Go container-registry default credential keychain; downloads support the Docker
`auths` files handled by `private/oci.bzl`. Credentials and tokens are never
recorded in facts. Credential-helper support differs between resolution and
downloading.

Facts assume published versions are immutable. Updating the chart version creates
a new resolution. To deliberately re-resolve an unchanged version, remove its
specific `helm-resolution-v1` entry from the extension's facts in
`MODULE.bazel.lock`, then run `bazel mod deps` with a fresh Bazel output base and
review the digest change. A fresh output base avoids restoring cached extension
results from Bazel's hidden lockfile.
`--lockfile_mode=refresh` is not a request to replace existing facts. Keep the
lockfile committed; missing facts require resolution even for an extension marked
reproducible.

**Breaking migration:** replace `helm_deps.lockfile(lockfile = ...)` with
`helm_deps.from_file(chart = ...)`. Remove `Chart.lock.json` and the BUILD targets
using `helm_lock_file` / `generate_lockfile`; those APIs have been removed.
`helm_deps.chart(...)` remains available for explicitly pinned dependencies.

### Developing the resolver

Consumers use a prebuilt, checksum-pinned `helmlock` binary; they do not need Go.
The binary exposes `inspect <Chart.yaml>` (offline parsing) and
`resolve <request.json>` (batched HTTP/OCI resolution), using JSON protocol 1.

For local development, build an executable and supply its absolute path:

```sh
go build -o "$PWD/helmlock-dev" ./cmd/helmlock
bazel mod deps --repo_env=RULES_HELM_RESOLVER="$PWD/helmlock-dev"
go test ./tools/extensiontest -resolver "$PWD/helmlock-dev" -count=1
```

The override is explicitly opt-in and bypasses the released helper checksum.
Without an override or a published pin, the extension reports an actionable
bootstrap error rather than executing an unpinned tool.

All Go tools and tests also have Bazel targets. Regenerate their BUILD files and
the Starlark libraries with the combined Gazelle generator:

```sh
bazel run //:gazelle
bazel run //:gazelle -- -mode=diff  # Check generated files without editing them.
bazel test //...
bazel build //cmd/helmlock //tools/pinhelmlock
```

`rules_go`, Gazelle, the Starlark Gazelle plugin, and Go dependency resolution are
development dependencies, not part of a consuming module's dependency graph.
The public root BUILD package does not load development-only tooling. The Bazel
integration test is opt-in (it starts a nested Bazel server and downloads Bazel
dependencies); enable it with `-resolver` as above, or pass `-resolver` and
`-rules-root` through `--test_arg` to `//tools/extensiontest:extensiontest_test`.

### Publishing releases

The GitHub workflows publish helper binaries for Linux and macOS, each on amd64
and arm64. Windows artifacts are not currently published.

1. Every push to `main` runs tests and uses **GoReleaser** (`.goreleaser.yaml`)
   to build four binaries, generate `SHA256SUMS`, and publish a prerelease named
   `v0.0.0-main.<full-commit-sha>`. Existing snapshot assets are never overwritten.
2. After that build succeeds, dispatch **Prepare release PR** on `main` with a
   version such as `0.5.0`. It selects the snapshot for the current main tip and
   opens `release/v0.5.0`, updating `MODULE.bazel` and `private/helmlock.json`.
   If the current main build is still pending or failed, preparation fails; wait
   for a successful build rather than implicitly selecting stale artifacts.
3. Review the selected artifact tag, source commit, version, and checksums. CI
   verifies all artifacts, checks that the helper source still matches the
   selected build, and exercises the extension using the published Linux binary.
   If helper source changes, close the stale release PR and prepare a fresh one
   by rerunning release preparation with the same version; the existing PR is
   updated using `peter-evans/create-pull-request`.
4. Merge the PR. **Publish merged release** tags the main commit as `v0.5.0` and
   publishes a stable release with the selected binaries, unchanged. The merge
   also gets the normal main prerelease, but that does not alter the selected
   release artifacts. Publication uses the merged PR's commit SHA, including for
   squash and rebase merges.

Enable Actions' **Allow GitHub Actions to create and approve pull requests** in
repository settings. Release preparation explicitly dispatches CI because PRs
created with `GITHUB_TOKEN` do not trigger `pull_request` workflows. Merge with a
human or GitHub App identity so the merge triggers the main push workflows.
No personal access token or GoReleaser Pro license is required. The only custom
release utility, `tools/pinhelmlock`, translates GoReleaser checksums into Bazel
metadata and verifies the promoted files; it does not build or publish releases.

For the first release, merge the workflow/helper changes, let the main snapshot
publish, then prepare the release PR. Until then, `private/helmlock.json`
intentionally has no artifact pin and local testing requires the explicit
resolver override. Release tags are created by the publishing workflow; do not
pre-create them or expect token-created tags to trigger another workflow.

For a Bazel MODULE example have a look in the [examples](./examples) folder.

```starlark
# jsonnet/BUILD.bazel
# Load jsonnet_to_json from your chosen rules_jsonnet distribution.
load("@rules_helm//:helm.bzl", "helm_template")

jsonnet_to_json(
    name = "cert-manager",
    src = "main.jsonnet",
    outs = ["manifests.yaml"],
    extra_args = ["-S"],
    tla_str_files = {
        ":cert_manager_template": "cert_manager_template",
    },
)

jsonnet_to_json(
    name = "values",
    src = "values.jsonnet",
    outs = ["vaules.yaml"],
    extra_args = ["-S"],
    deps = [
        "//jsonnet/lib/values/cert-manager",
    ],
)

helm_template(
    name = "cert_manager_template",
    chart = "@io_jetstack_charts_cert_manager//:chart",
    namespace = "cert-manager",
    values = ":values",
)
```
