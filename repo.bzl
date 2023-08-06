load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")
load("//:private/versions.bzl", "VERSIONS")
load("//:private/toolchain_repo.bzl", "toolchains_repo")

_BUILD_FILE_CONTENT = """
load("@rules_helm//:helm.bzl", "helm_toolchain")
package(default_visibility = ["//visibility:public"])

helm_toolchain(
    name = "helm_tools",
    src = ":helm",
)
"""

def helm_register_toolchains(name="sh_helm_get", version="3.10.2", register = True):
    current_version = VERSIONS[version]
    for platform in current_version:
        settings = current_version[platform]
        http_archive(
            name = "sh_helm_get_{}".format(platform),
            sha256 = settings["sha256"],
            urls = settings["urls"],
            strip_prefix = platform,
            build_file_content = _BUILD_FILE_CONTENT,
        )
        if register:
            native.register_toolchains("@{}_toolchains//:{}_toolchain".format(name, platform))

    toolchains_repo(
        name = name + "_toolchains",
        user_repository_name = name,
    )
