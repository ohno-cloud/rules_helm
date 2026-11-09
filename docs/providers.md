<!-- Generated with Stardoc: http://skydoc.bazel.build -->

Providers shared by Helm charts and toolchains.

<a id="HelmChartInfo"></a>

## HelmChartInfo

<pre>
load("@rules_helm//:providers.bzl", "HelmChartInfo")

HelmChartInfo(<a href="#HelmChartInfo-version">version</a>)
</pre>

Contains information about a Helm chart.

**FIELDS**

| Name  | Description |
| :------------- | :------------- |
| <a id="HelmChartInfo-version"></a>version |  The version of Helm chart.    |


<a id="HelmToolchainInfo"></a>

## HelmToolchainInfo

<pre>
load("@rules_helm//:providers.bzl", "HelmToolchainInfo")

HelmToolchainInfo(<a href="#HelmToolchainInfo-tool">tool</a>)
</pre>

Contains information about a Helm toolchain.

**FIELDS**

| Name  | Description |
| :------------- | :------------- |
| <a id="HelmToolchainInfo-tool"></a>tool |  The binary of Helm.    |


