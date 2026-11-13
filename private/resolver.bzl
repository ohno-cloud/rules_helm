"""Bootstrap the released, host-platform helmlock executable."""

def _resolver_impl(ctx):
    local = ctx.getenv("RULES_HELM_RESOLVER")
    executable = "helmlock.exe" if ctx.os.name.startswith("windows") else "helmlock"
    if local:
        path = ctx.path(local)
        if not path.exists:
            fail("RULES_HELM_RESOLVER does not exist: {}".format(local))
        ctx.watch(path)
        ctx.symlink(path, executable)
    else:
        metadata = json.decode(ctx.read(ctx.attr._metadata))
        os = {"mac os x": "darwin", "linux": "linux", "windows": "windows"}.get(ctx.os.name)
        arch = {"aarch64": "arm64", "arm64": "arm64", "amd64": "amd64", "x86_64": "amd64"}.get(ctx.os.arch)
        platform = "{}-{}".format(os, arch)
        digest = metadata["sha256"].get(platform)
        if not metadata["tag"] or not digest:
            fail("No released helmlock for {} is pinned yet. Publish a main prerelease and prepare a release PR, or set --repo_env=RULES_HELM_RESOLVER=/absolute/path/to/helmlock for development.".format(platform))
        ctx.download(
            url = "https://github.com/ohno-cloud/rules_helm/releases/download/{}/helmlock-{}{}".format(metadata["tag"], platform, ".exe" if os == "windows" else ""),
            output = executable,
            sha256 = digest,
            executable = True,
        )
    ctx.file("BUILD.bazel", 'exports_files(["{}"], visibility = ["//visibility:public"])\n'.format(executable))

    # A stable label for the extension, including on Windows.
    if executable != "helmlock":
        ctx.symlink(executable, "helmlock")
        ctx.file("BUILD.bazel", 'exports_files(["helmlock"], visibility = ["//visibility:public"])\n')

    # The development override points outside the repository and must stay local.
    # Released executables are pinned by digest; Bazel also keys repos by host OS/arch.
    return ctx.repo_metadata(reproducible = not local)

helmlock_repository = repository_rule(
    implementation = _resolver_impl,
    attrs = {"_metadata": attr.label(default = "//private:helmlock.json")},
)
