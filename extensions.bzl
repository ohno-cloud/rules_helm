load("//:repo.bzl", "helm_register_toolchains")

def _helm_configure_impl(ctx):
    helm_register_toolchains(register = False)

helm_configure = module_extension(implementation = _helm_configure_impl)
