<!-- Generated with Stardoc: http://skydoc.bazel.build -->

Public Helm chart, download, templating, inspection, and toolchain rules.

<a id="helm_chart"></a>

## helm_chart

<pre>
load("@rules_helm//:helm.bzl", "helm_chart")

helm_chart(<a href="#helm_chart-name">name</a>, <a href="#helm_chart-src">src</a>, <a href="#helm_chart-version">version</a>)
</pre>

Expose a local Helm chart file with its version for templating.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="helm_chart-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="helm_chart-src"></a>src |  The chart archive or file to expose.   | <a href="https://bazel.build/concepts/labels">Label</a> | optional |  `None`  |
| <a id="helm_chart-version"></a>version |  The chart version exposed through HelmChartInfo.   | String | required |  |


<a id="helm_show_crds"></a>

## helm_show_crds

<pre>
load("@rules_helm//:helm.bzl", "helm_show_crds")

helm_show_crds(<a href="#helm_show_crds-name">name</a>, <a href="#helm_show_crds-out">out</a>, <a href="#helm_show_crds-arguments">arguments</a>, <a href="#helm_show_crds-chart">chart</a>)
</pre>

Extract a chart's unrendered CRDs with helm show crds using the registered Helm toolchain. Charts without CRDs produce an empty file. This does not render templates or apply values.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="helm_show_crds-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="helm_show_crds-out"></a>out |  Output YAML file name. Defaults to `<target name>.crds.yaml`.   | String | optional |  `""`  |
| <a id="helm_show_crds-arguments"></a>arguments |  Additional arguments passed to helm show crds.   | List of strings | optional |  `[]`  |
| <a id="helm_show_crds-chart"></a>chart |  The chart archive to inspect, exposed by helm_repo_chart or helm_chart.   | <a href="https://bazel.build/concepts/labels">Label</a> | required |  |


<a id="helm_template"></a>

## helm_template

<pre>
load("@rules_helm//:helm.bzl", "helm_template")

helm_template(<a href="#helm_template-name">name</a>, <a href="#helm_template-out">out</a>, <a href="#helm_template-arguments">arguments</a>, <a href="#helm_template-chart">chart</a>, <a href="#helm_template-generate_name">generate_name</a>, <a href="#helm_template-include_crds">include_crds</a>, <a href="#helm_template-namespace">namespace</a>, <a href="#helm_template-skip_tests">skip_tests</a>,
              <a href="#helm_template-values">values</a>)
</pre>

Render a chart to a YAML manifest using the registered Helm toolchain and supplied values.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="helm_template-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="helm_template-out"></a>out |  The name of the output manifest file.   | String | optional |  `""`  |
| <a id="helm_template-arguments"></a>arguments |  Additional arguments to be passed to helm template.   | List of strings | optional |  `[]`  |
| <a id="helm_template-chart"></a>chart |  A helm chart to template.   | <a href="https://bazel.build/concepts/labels">Label</a> | required |  |
| <a id="helm_template-generate_name"></a>generate_name |  Sets the generation name used by helm - if not set uses label name instead.   | String | optional |  `""`  |
| <a id="helm_template-include_crds"></a>include_crds |  Sets the --include-crds flag.   | Boolean | optional |  `False`  |
| <a id="helm_template-namespace"></a>namespace |  Namespace to be set to.   | String | required |  |
| <a id="helm_template-skip_tests"></a>skip_tests |  Sets the --skip-tests flag.   | Boolean | optional |  `True`  |
| <a id="helm_template-values"></a>values |  A set of values to be used to template from.   | <a href="https://bazel.build/concepts/labels">Label</a> | optional |  `None`  |


<a id="helm_toolchain"></a>

## helm_toolchain

<pre>
load("@rules_helm//:helm.bzl", "helm_toolchain")

helm_toolchain(<a href="#helm_toolchain-name">name</a>, <a href="#helm_toolchain-src">src</a>)
</pre>

Wrap a Helm executable for use by helm_template and helm_show_crds.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="helm_toolchain-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="helm_toolchain-src"></a>src |  The helm binary   | <a href="https://bazel.build/concepts/labels">Label</a> | optional |  `None`  |


<a id="helm_repo_chart"></a>

## helm_repo_chart

<pre>
load("@rules_helm//:helm.bzl", "helm_repo_chart")

helm_repo_chart(<a href="#helm_repo_chart-name">name</a>, <a href="#helm_repo_chart-auth">auth</a>, <a href="#helm_repo_chart-chart">chart</a>, <a href="#helm_repo_chart-repository">repository</a>, <a href="#helm_repo_chart-sha256">sha256</a>, <a href="#helm_repo_chart-urls">urls</a>, <a href="#helm_repo_chart-version">version</a>, <a href="#helm_repo_chart-www_authenticate_challenges">www_authenticate_challenges</a>)
</pre>

Download a chart from HTTP archive URLs or an OCI repository and verify its SHA-256. Supply exactly one of urls or repository.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="helm_repo_chart-name"></a>name |  A unique name for this repository.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="helm_repo_chart-auth"></a>auth |  An optional dict specifying authentication information for some of the URLs.   | <a href="https://bazel.build/rules/lib/core/dict">Dictionary: String -> String</a> | optional |  `{}`  |
| <a id="helm_repo_chart-chart"></a>chart |  The chart name.   | String | required |  |
| <a id="helm_repo_chart-repository"></a>repository |  The OCI repository reference to pull the chart from.   | String | optional |  `""`  |
| <a id="helm_repo_chart-sha256"></a>sha256 |  The chart archive SHA-256: 64 hexadecimal characters, optionally prefixed with sha256:. Empty or invalid digests are rejected.   | String | required |  |
| <a id="helm_repo_chart-urls"></a>urls |  Direct URL to the Chart .tgz file.   | List of strings | optional |  `[]`  |
| <a id="helm_repo_chart-version"></a>version |  The exact chart version to download.   | String | required |  |
| <a id="helm_repo_chart-www_authenticate_challenges"></a>www_authenticate_challenges |  Optional registry host to WWW-Authenticate challenge mappings for OCI downloads.   | <a href="https://bazel.build/rules/lib/core/dict">Dictionary: String -> String</a> | optional |  `{}`  |


