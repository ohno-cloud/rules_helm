# Contributing

See [README.md](README.md) for using the rules. Development requires Bazel 9+
(preferably through Bazelisk); Bazel provides the Go SDK and other build tools.

## Build and test

Run these commands from the repository root:

```sh
bazel run //:gazelle
bazel test //...
```

Gazelle generates both Go BUILD targets and Starlark `bzl_library` targets. Commit
the generated BUILD changes alongside their sources. To check without editing:

```sh
bazel run //:gazelle -- -mode=diff
```

The full nested-Bazel integration test is opt-in. See its
[test instructions](tools/extensiontest/README.md) for running it with a local or
released resolver. With a local Go installation, `go test -race ./...` also runs
the unit tests with the race detector.

## API documentation

We use **Stardoc**, the successor to Skydoc. Edit public `.bzl` docstrings and
attribute `doc` fields, then regenerate the checked-in reference pages:

```sh
bazel build //docs:api
cp bazel-bin/docs/generated/*.md docs/
bazel test //docs:all
```

| Source | Generated reference |
| --- | --- |
| `helm.bzl` | `docs/rules.md` |
| `extensions.bzl` | `docs/extensions.md` |
| `providers.bzl` | `docs/providers.md` |

Commit the source changes and generated Markdown together. CI explicitly compares
the committed pages against fresh Stardoc output; it does not regenerate and
silently accept stale documentation. These checks also run in `bazel test //...`.

## Tooling and maintenance

- [Resolver development and local overrides](cmd/helmlock/README.md)
- [Bazel integration tests](tools/extensiontest/README.md)
- [Pinning and verifying released helpers](tools/pinhelmlock/README.md)
- [CI, release preparation, and publishing](.github/workflows/README.md)

Keep development dependencies marked `dev_dependency = True` in `MODULE.bazel`
and their BUILD loads outside the public root package. Put tool-specific
instructions beside the tool, configuration rationale in comments, and end-user
tasks in the README.
