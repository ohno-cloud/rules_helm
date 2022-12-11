load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

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

_VERSIONS = {
    "3.10.2": [
        {
            "os": "linux",
            "cpu": "x86_64",
            "prefix": "linux-amd64",
            "urls": ["https://get.helm.sh/helm-v3.10.2-linux-amd64.tar.gz"],
            "sha256": "2315941a13291c277dac9f65e75ead56386440d3907e0540bf157ae70f188347",
        },
        {
            "os": "linux",
            "cpu": "arm64",
            "prefix": "linux-arm64",
            "urls": ["https://get.helm.sh/helm-v3.10.2-linux-arm64.tar.gz"],
            "sha256": "57fa17b6bb040a3788116557a72579f2180ea9620b4ee8a9b7244e5901df02e4",
        },
        {
            "os": "osx",
            "cpu": "x86_64",
            "prefix": "darwin-amd64",
            "urls": ["https://get.helm.sh/helm-v3.10.2-darwin-amd64.tar.gz"],
            "sha256": "e889960e4c1d7e2dfdb91b102becfaf22700cb86dc3e3553d9bebd7bab5a3803",
        },
        {
            "os": "osx",
            "cpu": "arm64",
            "prefix": "darwin-arm64",
            "urls": ["https://get.helm.sh/helm-v3.10.2-darwin-arm64.tar.gz"],
            "sha256": "460441eea1764ca438e29fa0e38aa0d2607402f753cb656a4ab0da9223eda494",
        },
    ]
}


def helm_register_toolchains(version="3.10.2"):
    for helm in _VERSIONS[version]:
        http_archive(
            name = "sh_helm_get_{os}_{cpu}".format(os=helm['os'], cpu=helm['cpu']),
            sha256 = helm["sha256"],
            urls = helm["urls"],
            strip_prefix = helm["prefix"],
            build_file_content = _BUILD_FILE_CONTENT.format(
                bazel_cpu=helm['cpu'],
                bazel_os=helm['os'],
            ),
        )

        native.register_toolchains("@sh_helm_get_{os}_{cpu}//:toolchain".format(os=helm['os'], cpu=helm['cpu']))

