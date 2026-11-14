# Bazel integration tests

These opt-in Go tests start nested Bazel servers and local HTTP fixtures.
They check cold resolution, fact reuse, version changes, duplicate requests,
tampered downloads, repository naming, and conflicting declarations. They also
exercise Bazel's experimental remote repository contents cache with an in-process
HTTP cache and HTTP/OCI origins: a fresh workspace with local caches disabled must
reuse repositories while the origin is offline. Additional checks cover digest
validation, changed pins, credential-file invalidation, and removal of OCI tokens
and manifests from repository outputs. A resolver fixture verifies caching of
released executables and continued support for watched local overrides. No
external RBE service is needed.
The tests can download Bazel dependencies and must run locally, not through remote
execution.

Without `-resolver`, the tests are skipped by ordinary `bazel test //...` and
`go test ./...` runs. CI enables them explicitly for each supported Bazel version.
The HTTP remote-cache fixtures skip Bazel 9.1, whose repository caching is only
initialized for gRPC endpoints; they run on Bazel 9.2+.

The CRD extraction test builds real `helm_show_crds` actions with the pinned Helm
toolchain. It checks raw CRD output (without template rendering), charts without
CRDs, default/custom output names, paths containing spaces, additional arguments,
and rejection of inputs without `HelmChartInfo`.

## Run with Bazel

From the repository root, build a resolver and query its actual output path:

```sh
bazel build //cmd/helmlock
resolver="$PWD/$(bazel cquery --output=files //cmd/helmlock:helmlock)"

bazel test //tools/extensiontest:extensiontest_test \
  --test_arg="-resolver=$resolver" \
  --test_arg="-rules-root=$PWD" \
  --test_env=PATH \
  --nocache_test_results \
  --test_output=errors
```

The tests need `bazel` on `PATH`. `-rules-root` points to the source checkout used
by the nested workspace's `local_path_override`.

## Run with Go

With a local Go installation, run from the repository root:

```sh
go test ./tools/extensiontest -resolver "$resolver" -count=1
```

To test a released resolver instead, set `resolver` to that executable's absolute
path. For a different Bazel version, set `USE_BAZEL_VERSION` when using Bazelisk;
also forward it with `--test_env=USE_BAZEL_VERSION` for the Bazel test invocation.

Each run uses its own temporary workspace and sibling output/cache directories,
so caches do not sit inside the nested workspace. The tests shut down their nested
Bazel servers during cleanup.
