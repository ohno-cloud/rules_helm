load(":providers.bzl", "HelmChartInfo")

# ==============================================================================
# Helm repository chart
# ==============================================================================

def _helm_repo_chart(ctx):
    urls = ctx.attr.urls
    file_name = "{chart}-{version}.tgz".format(
        chart=ctx.attr.chart,
        version=ctx.attr.version,
        suffix=ctx.attr.chart_suffix,
    )
    # If `urls` are not given get some from the repository
    if len(urls) == 0:
        # FIXME: Only some repos are on the same domain
        # The {reposirory}/index.yaml is ensured to exist but the chart files are defined in
        # entries.{chart}[.version=${version}].urls
        urls.push("{repo}/{file_name}".format(
            repo=ctx.attr.repository,
            file_name=file_name,
        ))

    ctx.report_progress("Downloading from {}".format(urls))
    chart = ctx.download(
        url = urls,
        output = file_name,
        sha256 = ctx.attr.sha256,
        auth = ctx.attr.auth,
    )

    ctx.report_progress("Templating BUILD file.")
    ctx.template(
        "BUILD.bazel",
        ctx.attr._build_tpl,
        substitutions = {
            "{version}" : ctx.attr.version.replace('-', '_'),
            "{src}" : file_name,
        },
    )

_helm_repo_chart_attrs = {
    "repository": attr.string(
        doc = "The URL of the repository to pull the chart from.",
        mandatory = False,
    ),
    "chart": attr.string(
        doc = "The chart name.",
        mandatory = True,
    ),
    "version": attr.string(
        doc = "The repository URL to pull the chart from.",
        mandatory = True,
    ),

    "urls": attr.string_list(
        doc = "Direct URL to the Chart .tgz file.",
        mandatory = False,
    ),

    "sha256": attr.string(
        doc = "A SHA256 of the chart file.",
        mandatory = True,
    ),
    "chart_suffix": attr.string(
        doc = "A suffix that is added by the upstream repo to the chart name.",
        mandatory = False,
        default = "",
    ),
    "auth": attr.string_dict(
        doc = "An optional dict specifying authentication information for some of the URLs.",
        mandatory = False,
        default = {},
    ),
    "_build_tpl": attr.label(
        default = "//tools/helm_rules:repo.BUILD.bazel.tpl",
    ),
}

helm_repo_chart = repository_rule(
    implementation = _helm_repo_chart,
    attrs = dict(
        _helm_repo_chart_attrs.items()
    ),
    doc = "Depdend on an external Helm Chart that is published in a repostiory.",
)

# ==============================================================================
# Helm chart
# ==============================================================================

def _helm_chart(ctx):
    files = ctx.files.src

    return [
        DefaultInfo(files = depset(files)),
        HelmChartInfo(
            version=ctx.attr.version,
        ),
    ]

helm_chart = rule(
    implementation = _helm_chart,
    attrs = {
        "src": attr.label(
            allow_single_file=True,
        ),
        "version": attr.string(
            doc = "The repository URL to pull the chart from.",
            mandatory = True,
        ),
    }
)

# ==============================================================================
# Templating helm charts
# ==============================================================================

def _helm_template_impl(ctx):
    toolchain = ctx.toolchains["@rules_helm//:toolchain_type"]

    # Setup
    helm = toolchain.helm.tool[DefaultInfo].files_to_run
    info = ctx.attr.chart[HelmChartInfo]
    chart_path = ctx.attr.chart[DefaultInfo].files.to_list()[0].path
    suffix = hash("-".join([ctx.label.name, info.version]))

    output_name = "manifests-{}.yaml".format(suffix)
    manifests = ctx.actions.declare_file(output_name)

    args = [
        "template",
        "--namespace",
        ctx.attr.namespace,
        ctx.label.name,
    ]

    for value in ctx.attr.values.files.to_list():
        args.extend(["--values", value.path])

    args.append("./" + chart_path)

    inputs = depset(
        [helm.executable] +
        ctx.attr.chart.files.to_list() +
        ctx.attr.values.files.to_list(),
    )
    outputs = [manifests]

    # Do the helm chart template
    ctx.actions.run_shell(
        command = """
        $(pwd)/{0} {1} > {2}
        """.format(
            helm.executable.path,
            " ".join(args),
            manifests.path,
        ),
        mnemonic = "HelmTemplate",
        progress_message = "Helm tempalting {}".format(ctx.label),
        inputs = inputs,
        outputs = outputs,
        arguments = args,
    )

    return [DefaultInfo(
        files = depset(outputs),
    )]

_helm_template_attrs = {
    "chart": attr.label(
        doc = "A helm chart to template.",
        providers = [HelmChartInfo],
        allow_single_file = True,
    ),
    "values": attr.label(
        doc = "A set of values to be used to template from.",
        allow_single_file = True,
    ),
    "namespace": attr.string(
        doc = "Namespace to be set to.",
    ),
}

helm_template = rule(
    implementation = _helm_template_impl,
    attrs = dict(
        _helm_template_attrs.items()
    ),
    toolchains = ["@rules_helm//:toolchain_type"]
)

