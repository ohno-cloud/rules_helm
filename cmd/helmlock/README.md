# helmlock

The extension-time parser and resolver for Helm dependencies. Consumers download
a checksum-pinned executable; Go and Gazelle are development dependencies only.
Run the commands below from the repository root.

## Build and use a local resolver

```sh
bazel build //cmd/helmlock
resolver="$PWD/$(bazel cquery --output=files //cmd/helmlock:helmlock)"
(cd examples && bazel mod deps --repo_env=RULES_HELM_RESOLVER="$resolver")
```

Use the queried path rather than assuming a `bazel-bin` layout: Go transitions
can change the output directory. `RULES_HELM_RESOLVER` must be an absolute path;
it bypasses the released helper checksum and watches the local binary for changes.
Without an override or published pin, the extension reports a bootstrap error
rather than executing an unpinned tool.

Unit tests:

```sh
bazel test //cmd/helmlock:helmlock_test
```

For end-to-end testing, see the [integration test](../../tools/extensiontest/README.md).

## JSON protocol

The helper writes JSON protocol version 1 to stdout and errors to stderr:

- `helmlock inspect <Chart.yaml>` parses dependencies without network access or
  rewriting the input. The response has `protocol` and `dependencies` fields.
- `helmlock resolve <request.json>` batches HTTP repository index requests and OCI
  lookups. The response has `protocol` and `resolutions` fields, in request order.

A resolution request looks like:

```json
{
  "protocol": 1,
  "dependencies": [
    {
      "name": "cert-manager",
      "version": "v1.12.0",
      "repository": "https://charts.jetstack.io"
    }
  ]
}
```

Only exact versions and HTTP(S)/OCI repository URLs are supported. HTTP resolution
returns archive URLs and SHA-256. OCI resolution returns repository, chart,
version, and the Helm content-layer archive digest, not a manifest digest.
OCI resolution uses the Go container-registry default credential keychain.

`extensions.bzl` owns fact lookup and persistence: the helper only receives
dependencies whose resolution is missing. Do not persist credentials or token
responses, and version any incompatible changes to the protocol or facts schema.

## Replacing an existing resolution

Facts assume published versions are immutable. Prefer updating the chart version.
If an unchanged version must deliberately be re-resolved, remove only its
`helm-resolution-v1` fact from `MODULE.bazel.lock`, resolve with a fresh Bazel
output base, and review the digest change. The fresh output base avoids restoring
cached extension results from Bazel's hidden lockfile.

`--lockfile_mode=refresh` does not replace existing facts. Keep the lockfile
committed; missing facts require resolution even for an extension marked
reproducible.

For publishing the executable, see the [release workflows](../../.github/workflows/README.md#releases).
