# CI and releases

## CI

[`ci.yml`](ci.yml) tests Bazel 9.0.0 and 9.3.2. It runs Go race tests, checks
Gazelle output, explicitly checks Stardoc freshness with `bazel test //docs:all`,
and runs the Bazel suite and opt-in integration test. Release-preparation PRs are
also tested with the published helper binary they select.

The Stardoc checks compare committed reference pages against fresh, unstamped
output. Update them using the [contribution instructions](../../CONTRIBUTING.md#api-documentation),
not by editing generated Markdown directly.

## Releases

GoReleaser ([configuration](../../.goreleaser.yaml)) builds helpers for Linux and
macOS on amd64 and arm64. Windows artifacts are not currently published. The Go
[pinning utility](../../tools/pinhelmlock/README.md) only translates and verifies
checksum pins; it does not build or publish releases.

1. Each push to `main` runs [`prerelease.yml`](prerelease.yml) and publishes
   `v0.0.0-main.<full-commit-sha>`, containing four binaries and `SHA256SUMS`.
   Existing snapshot assets are never overwritten.
2. After that build succeeds, dispatch **Prepare release PR**
   ([`release-prep.yml`](release-prep.yml)) on `main` with a version such as
   `0.5.0`. It selects the snapshot for the current main tip and opens
   `release/v0.5.0`, updating `MODULE.bazel` and `private/helmlock.json`.
   Preparation fails if the current snapshot is not available; wait for its
   successful build rather than implicitly selecting stale artifacts.
3. Review the artifact tag, source commit, version, and checksums. CI verifies
   the artifacts, checks that their helper source still matches the PR, and
   tests the published Linux binary. If helper source changes, rerun release
   preparation with the same version to refresh the existing PR.
4. Merge the PR. **Publish merged release** ([`release.yml`](release.yml)) tags
   the merged commit as `v0.5.0` and publishes the selected binaries unchanged,
   without rebuilding. The merge also gets a normal main snapshot, but that
   does not alter the selected assets. The workflow uses the merged PR's commit
   SHA, including squash/rebase merges.

Enable Actions' **Allow GitHub Actions to create and approve pull requests** in
repository settings. Preparation explicitly dispatches CI because PRs created
with `GITHUB_TOKEN` do not trigger `pull_request` workflows. Merge with a human or
GitHub App identity so the merge triggers the release and main-push workflows.
No personal access token or GoReleaser Pro license is required.

### Bootstrapping the first artifacts

Until the first snapshot has been published and pinned, `private/helmlock.json`
has no artifact pin and local testing requires `RULES_HELM_RESOLVER`. The
prerelease workflow temporarily also allows pushes to `mctx-facts` to produce the
initial artifacts. Its snapshot tag format is unchanged so the pinning utility
can consume them. Remove that temporary branch allowance after bootstrapping.

Release tags are created by the publishing workflows. Do not pre-create them or
expect tags created with `GITHUB_TOKEN` to trigger another workflow.
