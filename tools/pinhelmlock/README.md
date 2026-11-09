# pinhelmlock

Translate GoReleaser's `SHA256SUMS` into the Bazel resolver pins in
`private/helmlock.json`, or verify downloaded artifacts against those pins.
This tool does not build binaries or publish releases; the
[release workflows](../../.github/workflows/README.md#releases) normally invoke it.

Run commands from the repository root, after downloading a snapshot's four
`helmlock-<os>-<arch>` binaries and `SHA256SUMS` into an artifacts directory:

```sh
bazel run //tools/pinhelmlock -- pin \
  --artifacts /absolute/path/to/artifacts \
  --tag "v0.0.0-main.<full-commit-sha>" \
  --version 0.5.0

bazel run //tools/pinhelmlock -- verify \
  --artifacts /absolute/path/to/artifacts
```

Under `bazel run`, the tool uses `BUILD_WORKSPACE_DIRECTORY` to read and update
the checkout; relative artifact paths are also resolved from the workspace root.

`pin` checks every binary's SHA-256, records the snapshot tag and source commit,
and updates the rules version in `MODULE.bazel`. `verify` checks the downloaded
files against both the checksum manifest and the pins, and checks that the
rules version matches `MODULE.bazel`. Neither command accepts missing platforms,
duplicate checksum entries, unexpected filenames, or checksum mismatches.

The supported platforms are Linux and macOS, each on amd64 and arm64. Snapshot
tags must be `v0.0.0-main.<40-character-commit-sha>`; release versions have no
leading `v`. The workflows additionally check that helper sources still match
the selected build before publishing.

```sh
bazel test //tools/pinhelmlock:pinhelmlock_test
```
