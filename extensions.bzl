load("//:repo.bzl", "helm_register_toolchains")
load("//:helm.bzl", "helm_repo_chart")

def _helm_configure_impl(ctx):
    helm_register_toolchains(register = False)

_version = tag_class(attrs = {"version": attr.string() })

helm_configure = module_extension(
    implementation = _helm_configure_impl,
    tag_classes = {"version": _version},
)

def _helm_deps(ctx):
    # collect artifacts from across the dependency graph
    targets = {}
    for mod in ctx.modules:
        for lockfile in mod.tags.lockfile:
            buf = ctx.read(lockfile.lockfile)
            lockfile = json.decode(buf)
            targets.update(lockfile["repositories"])

        for chart in mod.tags.chart:
           targets[chart.name] = {
            'chart': chart.chart,
            'version': chart.version,
            'urls': chart.urls,
            'digest': chart.digest,
           }

    for key, val in targets.items():
        helm_repo_chart(
            name = key,
            chart = val['chart'],
            version = val['version'],
            urls = val['urls'],
            sha256 = val['digest'],
        )

_lockfile = tag_class(attrs = {"lockfile": attr.label() })
_chart = tag_class(attrs = {
    "name": attr.string(),
    "urls": attr.string_list(),
    "digest": attr.string(),
    "chart": attr.string(),
    "version": attr.string(),
})

helm_deps = module_extension(
    implementation = _helm_deps,
    tag_classes = {"lockfile": _lockfile, "chart": _chart},
)

