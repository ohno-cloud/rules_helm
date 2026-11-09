"""Helm toolchains and facts-backed dependency resolution for Bazel 9+."""

load("//:helm.bzl", "helm_repo_chart")
load("//:repo.bzl", "helm_register_toolchains")

def _helm_configure_impl(_ctx):
    helm_register_toolchains(register = False)

_version = tag_class(attrs = {"version": attr.string(doc = "Reserved for future version selection; the default Helm toolchain version is currently used.")})

helm_configure = module_extension(
    implementation = _helm_configure_impl,
    doc = "Declare the platform-specific Helm toolchain repositories.",
    tag_classes = {"version": _version},
)

_RESOLVER = Label("@rules_helm_resolver//:helmlock")
_PROTOCOL = 1

def _fact_key(repository, chart, version):
    # Bazel shallow-merges facts and retains them across extension changes.
    # Use immutable artifact identities with a schema namespace, not generated
    # repository names, which can change when more versions are requested.
    return json.encode(["helm-resolution-v1", repository, chart, version])

def _run_resolver(ctx, command, input):
    result = ctx.execute([ctx.path(_RESOLVER), command, input], timeout = 360)
    if result.return_code:
        fail("helmlock {} failed: {}{}".format(command, result.stdout, result.stderr))
    response = json.decode(result.stdout)
    if response.get("protocol") != _PROTOCOL:
        fail("unsupported helmlock protocol: {}".format(response.get("protocol")))
    return response

def _repo_name(dep, multiple):
    repository = dep["repository"]
    authority_path = repository.split("://", 1)[1].split("/", 1)
    host = authority_path[0].split(":", 1)[0].replace("-", "_").split(".")
    parts = list(reversed(host))
    if repository.startswith("oci://") and len(authority_path) == 2:
        parts.extend([part.replace("-", "_") for part in authority_path[1].strip("/").split("/") if part])
    parts.extend(dep["name"].split("-"))
    if multiple:
        parts.extend(dep["version"].replace("-", "_").replace("+", "_").split("."))
    return "_".join(parts)

def _add_target(targets, name, value):
    if name in targets and targets[name] != value:
        fail("conflicting Helm dependencies produce repository '{}'; use distinct repositories or explicit chart declarations".format(name))
    targets[name] = value

def _validate_resolution(key, value):
    if _fact_key(value["repository"], value["chart"], value["version"]) != key:
        fail("Helm resolution identity does not match fact key {}".format(key))
    digest = value.get("digest", "")
    if not digest.startswith("sha256:") or len(digest) != 71 or any([c not in "0123456789abcdef" for c in digest[7:].elems()]):
        fail("invalid Helm chart SHA-256 in fact {}".format(key))
    if not value["repository"].startswith("oci://") and not value.get("urls"):
        fail("HTTP Helm fact has no archive URLs: {}".format(key))

def _helm_deps(ctx):
    dependencies = {}
    owners = {}
    manual = []
    for mod in ctx.modules:
        for tag in mod.tags.from_file:
            # Read through Bazel before invoking the parser to track source changes.
            ctx.report_progress("Reading Helm dependencies from {}".format(tag.chart))
            content = ctx.read(tag.chart)
            ctx.file("Chart.input.yaml", content)
            response = _run_resolver(ctx, "inspect", "Chart.input.yaml")
            for dep in response.get("dependencies", []):
                key = _fact_key(dep["repository"], dep["name"], dep["version"])
                dependencies[key] = dep
                if mod.is_root:
                    owners.setdefault(key, []).append(ctx.is_dev_dependency(tag))
        for tag in mod.tags.chart:
            manual.append((mod, tag))

    facts = {}
    missing = []
    versions = {}
    for key, dep in dependencies.items():
        value = ctx.facts.get(key)
        if value != None:
            _validate_resolution(key, value)
            facts[key] = value
        else:
            missing.append(dep)
        identity = json.encode([dep["repository"], dep["name"]])
        versions.setdefault(identity, []).append(dep["version"])

    if missing:
        ctx.report_progress("Resolving {} Helm dependencies".format(len(missing)))
        ctx.file("resolve.input.json", json.encode({"protocol": _PROTOCOL, "dependencies": missing}))
        response = _run_resolver(ctx, "resolve", "resolve.input.json")
        for value in response.get("resolutions", []):
            key = _fact_key(value["repository"], value["chart"], value["version"])
            if key not in dependencies or key in facts:
                fail("helmlock returned an unexpected or duplicate resolution: {}".format(key))
            _validate_resolution(key, value)
            facts[key] = value

    targets = {}
    direct = {}
    for key, dep in dependencies.items():
        if key not in facts:
            fail("helmlock did not resolve {}".format(key))
        identity = json.encode([dep["repository"], dep["name"]])
        name = _repo_name(dep, len(versions[identity]) > 1)
        _add_target(targets, name, facts[key])
        if key in owners:
            direct[name] = direct.get(name, True) and all(owners[key])

    for mod, tag in manual:
        value = {
            "chart": tag.chart,
            "version": tag.version,
            "urls": tag.urls,
            "repository": tag.repository,
            "digest": tag.digest,
        }
        if tag.repository and not tag.repository.startswith("oci://"):
            fail("helm_deps.chart repository must use oci://; use urls for HTTP archives")
        _validate_resolution(_fact_key(tag.repository, tag.chart, tag.version), value)
        _add_target(targets, tag.name, value)
        if mod.is_root:
            direct[tag.name] = direct.get(tag.name, True) and ctx.is_dev_dependency(tag)

    for name, value in targets.items():
        helm_repo_chart(
            name = name,
            chart = value["chart"],
            version = value["version"],
            urls = value.get("urls", []),
            repository = value["repository"] if value["repository"].startswith("oci://") else "",
            sha256 = value["digest"],
        )

    # Facts pin resolution metadata; the repository rules enforce archive hashes.
    # Reproducible extensions still persist facts, without generated repo specs.
    return ctx.extension_metadata(
        reproducible = True,
        facts = facts,
        root_module_direct_deps = [name for name, dev in direct.items() if not dev],
        root_module_direct_dev_deps = [name for name, dev in direct.items() if dev],
    )

_from_file = tag_class(
    doc = "Read exact chart dependencies from an ordinary Chart.yaml, resolving only entries missing from Bazel facts.",
    attrs = {"chart": attr.label(mandatory = True, doc = "Source Chart.yaml containing Helm dependency declarations.")},
)
_chart = tag_class(attrs = {
    "name": attr.string(mandatory = True, doc = "Explicit Bazel repository name for the chart."),
    "urls": attr.string_list(doc = "HTTP(S) URLs of the chart archive; mutually exclusive with repository."),
    "repository": attr.string(doc = "OCI repository URL without the chart name; mutually exclusive with urls."),
    "digest": attr.string(mandatory = True, doc = "Chart archive SHA-256, prefixed with sha256:."),
    "chart": attr.string(mandatory = True, doc = "Published chart name."),
    "version": attr.string(mandatory = True, doc = "Exact chart version."),
})

helm_deps = module_extension(
    implementation = _helm_deps,
    doc = "Expose HTTP and OCI Helm charts as Bazel repositories. from_file dependencies are pinned per repository, chart, and version in MODULE.bazel.lock facts; chart tags supply explicit pins.",
    tag_classes = {"from_file": _from_file, "chart": _chart},
)
