load(":providers.bzl", "HelmChartInfo")

# ==============================================================================
# Helm repository chart
# ==============================================================================

def _helm_repo_chart(ctx):
    urls = ctx.attr.urls
    file_name = "{chart}-{version}.tgz".format(
        chart=ctx.attr.chart,
        version=ctx.attr.version,
    )

    if len(urls) > 0 and ctx.attr.repository:
        fail("helm_repo_chart expects exactly one of 'urls' or 'repository'")

    if len(urls) == 0 and not ctx.attr.repository:
        fail("helm_repo_chart requires either 'urls' or 'repository'")

    if len(urls) > 0:
        ctx.report_progress("Downloading from {}".format(urls))
        ctx.download(
            url = urls,
            output = file_name,
            sha256 = _strip_sha256_prefix(ctx.attr.sha256),
            auth = ctx.attr.auth,
        )
    else:
        if not ctx.attr.repository.startswith("oci://"):
            fail("helm_repo_chart only accepts repository values with the oci:// scheme")

        helm = ctx.which("helm")
        if helm == None:
            fail("helm binary not found in PATH; OCI charts require helm pull and helm registry login credentials")

        pull = ctx.execute([
            helm,
            "pull",
            "{}/{}".format(ctx.attr.repository, ctx.attr.chart),
            "--version",
            ctx.attr.version,
            "--destination",
            ctx.path("."),
        ])
        if pull.return_code != 0:
            fail("failed to pull OCI chart {}: {}{}".format(ctx.attr.repository, pull.stdout, pull.stderr))

        digest_result = ctx.execute([
            "shasum",
            "-a",
            "256",
            file_name,
        ])
        if digest_result.return_code != 0:
            fail("failed to calculate sha256 for {}: {}{}".format(file_name, digest_result.stdout, digest_result.stderr))

        actual_digest = "sha256:{}".format(digest_result.stdout.split(" ", 1)[0].strip())
        if actual_digest != ctx.attr.sha256:
            fail("sha256 mismatch for {}: got {}, want {}".format(file_name, actual_digest, ctx.attr.sha256))

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
        doc = "The OCI repository reference to pull the chart from.",
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
    "auth": attr.string_dict(
        doc = "An optional dict specifying authentication information for some of the URLs.",
        mandatory = False,
        default = {},
    ),
    "_build_tpl": attr.label(
        default = "//:repo.BUILD.bazel.tpl",
    ),
}

helm_repo_chart = repository_rule(
    implementation = _helm_repo_chart,
    attrs = dict(
        _helm_repo_chart_attrs.items()
    ),
    doc = "Depdend on an external Helm Chart that is published in a repostiory.",
)

def _strip_sha256_prefix(value):
    if value.startswith("sha256:"):
        return value[len("sha256:"):]
    return value

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
    toolchain = ctx.toolchains["//:toolchain_type"]

    # Setup
    helm = toolchain.helm.tool[DefaultInfo].files_to_run
    info = ctx.attr.chart[HelmChartInfo]
    chart_path = ctx.attr.chart[DefaultInfo].files.to_list()[0].path
    suffix = hash("-".join([ctx.label.name, info.version]))

    output_name = "manifests-{}-{}.yaml".format(ctx.label.name, suffix)
    if ctx.attr.out:
        output_name = ctx.attr.out
    manifests = ctx.actions.declare_file(output_name)

    generate_name = ctx.label.name
    if ctx.attr.generate_name != "":
        generate_name = ctx.attr.generate_name

    args = [
        "template",
        generate_name,
        "./" + chart_path,
        "--namespace",
        ctx.attr.namespace,
    ]

    if ctx.attr.include_crds:
        args.append("--include-crds")

    if ctx.attr.skip_tests:
        args.append("--skip-tests")

    args.extend(ctx.attr.arguments)

    for value in ctx.attr.values.files.to_list():
        args.extend(["--values", value.path])

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
    return [DefaultInfo(files = depset(outputs))]

_helm_template_attrs = {
    "chart": attr.label(
        doc = "A helm chart to template.",
        providers = [HelmChartInfo],
        allow_single_file = True,
        mandatory = True,
    ),
    "generate_name": attr.string(
        doc = "Sets the generation name used by helm - if not set uses label name instead.",
        mandatory = False,
    ),
    "out": attr.string(
        doc = "The name of the output manifest file.",
        mandatory = False,
    ),
    "values": attr.label(
        doc = "A set of values to be used to template from.",
        allow_single_file = True,
    ),
    "include_crds": attr.bool(
        doc = "Sets the --include-crds flag.",
        mandatory = False,
        default = False,
    ),
    "skip_tests": attr.bool(
        doc = "Sets the --skip-tests flag.",
        mandatory = False,
        default = True,
    ),
    "namespace": attr.string(
        doc = "Namespace to be set to.",
        mandatory = True,
    ),
    "arguments": attr.string_list(
        doc = "Additional arguments to be passed to helm template.",
        mandatory = False,
    )
}

helm_template = rule(
    implementation = _helm_template_impl,
    attrs = dict(
        _helm_template_attrs.items()
    ),
    toolchains = ["//:toolchain_type"]
)
