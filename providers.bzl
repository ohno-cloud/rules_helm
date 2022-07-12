HelmChartInfo = provider(
    doc = "Contains information about a Helm chart.",
    fields = {
        "version": "The version of Helm chart.",
    }
)

HelmToolchainInfo = provider(
    doc = "Contains information about a Helm toolchain.",
    fields = {
        "tool": "The binary of Helm.",
    }
)
