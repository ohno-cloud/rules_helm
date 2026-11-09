# Bazel integration test

This opt-in Go test starts a nested Bazel server and a local HTTP Helm repository.
It checks cold resolution, fact reuse, version changes, duplicate requests,
tampered downloads, repository naming, and conflicting declarations. It can
download Bazel dependencies and must run locally, not through remote execution.

Without `-resolver`, the test is skipped by ordinary `bazel test //...` and
`go test ./...` runs. CI enables it explicitly for each supported Bazel version.

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

The test needs `bazel` on `PATH`. `-rules-root` points to the source checkout used
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
so caches do not sit inside the nested workspace. The test shuts down its nested
Bazel server during cleanup.
