load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

_VERSION = "v3.9.0"

_BUILD_FILE_CONTENT = """
load("@rules_helm//:helm.bzl", "helm_toolchain")
package(default_visibility = ["//visibility:public"])

helm_toolchain(
    name = "helm_tools",
    src = ":helm",
)

toolchain(
    name = "toolchain",
    exec_compatible_with = [
        "@platforms//os:{bazel_os}",
        "@platforms//cpu:{bazel_cpu}",
    ],
    target_compatible_with = [
        "@platforms//os:{bazel_os}",
        "@platforms//cpu:{bazel_cpu}",
    ],
    toolchain = ":helm_tools",
    toolchain_type = "@rules_helm//:toolchain_type",
)
"""

def helm_register_toolchains():
    for helm in [
    {
        "os": "linux",
        "cpu": "amd64",
        "bazel_cpu": "x86_64",
        "sha256": "1484ffb0c7a608d8069470f48b88d729e88c41a1b6602f145231e8ea7b43b50a",
    },
    {
        "os": "darwin",
        "cpu": "amd64",
        "bazel_cpu": "x86_64",
        "bazel_os": "osx",
        "sha256": "7e5a2f2a6696acf278ea17401ade5c35430e2caa57f67d4aa99c607edcc08f5e",
    }
    ]:

        http_archive(
            name = "sh_helm_get_{os}_{cpu}".format(os=helm['os'], cpu=helm['cpu']),
            sha256 = helm["sha256"],
            urls = ["https://get.helm.sh/helm-{version}-{os}-{cpu}.tar.gz"
                .format(
                    version=_VERSION,
                    os=helm['os'],
                    cpu=helm['cpu'],
                )],
            strip_prefix = "{os}-{cpu}".format(os=helm['os'], cpu=helm['cpu']),
            build_file_content = _BUILD_FILE_CONTENT.format(
                bazel_cpu=helm.get('bazel_cpu', helm['cpu']),
                bazel_os=helm.get('bazel_os', helm['os']),
                cpu=helm['cpu'],
                os=helm['os'],
            ),
        )

        native.register_toolchains("@sh_helm_get_{os}_{cpu}//:toolchain".format(os=helm['os'], cpu=helm['cpu']))

