# `helm_rules`

## Rules

 * `helm_chart` - Basic wrapper around files to become a "Helm Chart"
 * `helm_repo_chart` - Pulls a chart from a repository & validates it with sha256
 * `helm_template` - Templates the chart with a given set of `values.yaml`

## Examples


```starlark
# WORKSPACE

http_archive(
    name = "rules_helm",
    sha256 = "< curl -L url | sha256sum | cut -d' ' -f1 >",
    strip_prefix = "rules_helm-<COMMIT SHA>",
    urls = ["https://github.com/dbanetto/rules_helm/archive/<COMMIT SHA>.tar.gz"],
)

load("@helm_rules//:repo.bzl", "helm_repositories")

helm_repositories(register_toolchains=register_toolchains)

load("//third-party/helm:deps.bzl", "helm_dependencies")

helm_dependencies()

# thrid-part/helm/deps.bzl

load("//tools/helm_rules:helm.bzl", "helm_repo_chart")

def helm_dependencies():

    helm_repo_chart(
        name = "io_jetstack_charts_cert_manager",
        chart = "cert-manager",
        version = "1.8.2",
        urls = ["https://charts.jetstack.io/charts/cert-manager-v1.8.2.tgz"],
        sha256 = "3e3262f08455d02f025e803b8227dea3ff3f88c170bfb0655b513fa274de5592",
    )

# jsonnet/BUILD.bazel

jsonnet_to_json(
    name = "cert-manager",
    src = "main.jsonnet",
    outs = ["manifests.yaml"],
    extra_args = ["-S"],
    tla_str_files = {
        ":cert_manager_template": "cert_manager_template",
    },
)

jsonnet_to_json(
    name = "values",
    src = "values.jsonnet",
    outs = ["vaules.yaml"],
    extra_args = ["-S"],
    deps = [
        "//jsonnet/lib/values/cert-manager",
    ],
)

helm_template(
    name = "cert_manager_template",
    chart = "@io_jetstack_charts_cert_manager//:chart",
    namespace = "cert-manager",
    values = ":values",
)
```
