_OCI_ACCEPT_HEADER = ", ".join([
    "application/vnd.oci.image.manifest.v1+json",
    "application/vnd.docker.distribution.manifest.v2+json",
    "application/vnd.oci.image.index.v1+json",
    "application/vnd.docker.distribution.manifest.list.v2+json",
])

_HELM_CHART_LAYER_MEDIA_TYPE = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"

_DEFAULT_WWW_AUTHENTICATE_CHALLENGES = {
    "index.docker.io": 'Bearer realm="https://auth.docker.io/token",service="registry.docker.io"',
    "registry-1.docker.io": 'Bearer realm="https://auth.docker.io/token",service="registry.docker.io"',
    "public.ecr.aws": 'Bearer realm="https://public.ecr.aws/token",service="public.ecr.aws"',
    "ghcr.io": 'Bearer realm="https://ghcr.io/token",service="ghcr.io"',
    "quay.io": 'Bearer realm="https://quay.io/v2/auth",service="quay.io"',
    "registry.gitlab.com": 'Bearer realm="https://gitlab.com/jwt/auth",service="container_registry"',
}

def download_oci_chart(ctx, repository, chart, version, output):
    oci = _parse_oci_repository(repository, chart)
    challenge = _get_www_authenticate_challenge(ctx, oci.registry)
    headers = _oci_auth_headers(ctx, oci, challenge)

    manifest_file = "{}.manifest.json".format(chart)
    manifest_result = ctx.download(
        url = _oci_manifest_url(oci, version),
        output = manifest_file,
        allow_fail = True,
        headers = dict(headers, Accept = _OCI_ACCEPT_HEADER),
    )
    if not manifest_result.success:
        fail("failed to fetch OCI manifest for {}:{}: {}".format(oci.reference, version, manifest_result.error))

    manifest = json.decode(ctx.read(manifest_file))
    layer = _find_helm_chart_layer(manifest)
    if layer == None:
        fail("did not find helm chart layer in OCI manifest for {}:{}".format(oci.reference, version))

    blob_result = ctx.download(
        url = _oci_blob_url(oci, layer["digest"]),
        output = output,
        allow_fail = True,
        headers = headers,
    )
    if not blob_result.success:
        fail("failed to download OCI chart blob for {}:{}: {}".format(oci.reference, version, blob_result.error))

def _parse_oci_repository(repository, chart):
    stripped = repository[len("oci://"):]
    parts = stripped.split("/", 1)
    if len(parts) != 2:
        fail("invalid OCI repository '{}': expected oci://<registry>/<path>".format(repository))

    path = parts[1].strip("/")
    if path == "":
        fail("invalid OCI repository '{}': missing repository path".format(repository))

    full_repository = "{}/{}".format(path, chart)
    return struct(
        registry = parts[0],
        repository = full_repository,
        reference = "{}/{}".format(parts[0], full_repository),
    )

def _get_www_authenticate_challenge(ctx, registry):
    if registry in ctx.attr.www_authenticate_challenges:
        return ctx.attr.www_authenticate_challenges[registry]
    return _DEFAULT_WWW_AUTHENTICATE_CHALLENGES.get(registry, "")

def _oci_auth_headers(ctx, oci, challenge):
    if not challenge:
        return {}

    params = _parse_www_authenticate_challenge(challenge)
    if params.get("scheme", "") != "Bearer":
        fail("unsupported OCI auth scheme '{}' for registry {}".format(params.get("scheme", ""), oci.registry))

    realm = params.get("realm", "")
    if not realm:
        fail("missing realm in OCI auth challenge for registry {}".format(oci.registry))

    query = ["scope=repository:{}:pull".format(oci.repository)]
    if params.get("service", ""):
        query.append("service={}".format(params["service"]))

    token_file = "{}.token.json".format(oci.registry.replace(":", "_"))
    token_headers = _basic_auth_headers_for_registry(ctx, oci.registry)
    token_result = ctx.download(
        url = "{}?{}".format(realm, "&".join(query)),
        output = token_file,
        allow_fail = True,
        headers = token_headers,
    )
    if not token_result.success:
        fail("failed to fetch OCI auth token for registry {}: {}".format(oci.registry, token_result.error))

    token_response = json.decode(ctx.read(token_file))
    token = token_response.get("token", token_response.get("access_token", ""))
    if not token:
        fail("OCI auth token response for registry {} did not contain a token".format(oci.registry))

    return {"Authorization": "Bearer {}".format(token)}

def _basic_auth_headers_for_registry(ctx, registry):
    docker_config = _read_docker_config(ctx)
    if docker_config == None:
        return {}

    auths = docker_config.get("auths", {})
    candidates = [registry, "https://{}".format(registry)]
    for key in candidates:
        entry = auths.get(key)
        if entry and entry.get("auth", ""):
            return {"Authorization": "Basic {}".format(entry["auth"])}
    return {}

def _read_docker_config(ctx):
    env = ctx.os.environ
    for candidate in _auth_file_candidates(env):
        config_file = ctx.path(candidate)
        if config_file.exists:
            return json.decode(ctx.read(config_file))

    return None

def _auth_file_candidates(env):
    candidates = []

    if env.get("REGISTRY_AUTH_FILE", ""):
        candidates.append(env["REGISTRY_AUTH_FILE"])

    if env.get("XDG_RUNTIME_DIR", ""):
        candidates.append("{}/containers/auth.json".format(env["XDG_RUNTIME_DIR"]))

    home = env.get("HOME", "")
    if home:
        candidates.extend([
            "{}/.config/containers/auth.json".format(home),
            "{}/.local/share/containers/auth.json".format(home),
        ])

    if env.get("DOCKER_CONFIG", ""):
        candidates.append("{}/config.json".format(env["DOCKER_CONFIG"]))
    elif home:
        candidates.append("{}/.docker/config.json".format(home))

    return candidates

def _parse_www_authenticate_challenge(challenge):
    parts = challenge.split(" ", 1)
    if len(parts) == 1:
        return {"scheme": parts[0]}

    params = {"scheme": parts[0]}
    for item in parts[1].split(","):
        key, value = item.split("=", 1)
        params[key.strip()] = value.strip().strip('"')
    return params

def _find_helm_chart_layer(manifest):
    for layer in manifest.get("layers", []):
        if layer.get("mediaType", "") == _HELM_CHART_LAYER_MEDIA_TYPE:
            return layer
    return None

def _oci_manifest_url(oci, version):
    return "https://{}/v2/{}/manifests/{}".format(oci.registry, oci.repository, version)

def _oci_blob_url(oci, digest):
    return "https://{}/v2/{}/blobs/{}".format(oci.registry, oci.repository, digest)
