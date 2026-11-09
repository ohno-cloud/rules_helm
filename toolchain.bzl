"""Helm executable toolchain implementation."""

load(":providers.bzl", "HelmToolchainInfo")

def _helm_toolchain(ctx):
    return [
        platform_common.ToolchainInfo(
            helm = HelmToolchainInfo(
                tool = ctx.attr.src,
            ),
        ),
    ]

helm_toolchain = rule(
    implementation = _helm_toolchain,
    doc = "Wrap a Helm executable for use by helm_template's toolchain.",
    attrs = {
        "src": attr.label(
            doc = "The helm binary",
            cfg = "exec",
            executable = True,
            allow_single_file = True,
        ),
    },
)
