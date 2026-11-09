<!-- Generated with Stardoc: http://skydoc.bazel.build -->

Helm toolchains and facts-backed dependency resolution for Bazel 9+.

<a id="helm_configure"></a>

## helm_configure

<pre>
helm_configure = use_extension("@rules_helm//:extensions.bzl", "helm_configure")
helm_configure.version(<a href="#helm_configure.version-version">version</a>)
</pre>

Declare the platform-specific Helm toolchain repositories.


**TAG CLASSES**

<a id="helm_configure.version"></a>

### version

**Attributes**

| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="helm_configure.version-version"></a>version |  Reserved for future version selection; the default Helm toolchain version is currently used.   | String | optional |  `""`  |


<a id="helm_deps"></a>

## helm_deps

<pre>
helm_deps = use_extension("@rules_helm//:extensions.bzl", "helm_deps")
helm_deps.from_file(<a href="#helm_deps.from_file-chart">chart</a>)
helm_deps.chart(<a href="#helm_deps.chart-name">name</a>, <a href="#helm_deps.chart-chart">chart</a>, <a href="#helm_deps.chart-digest">digest</a>, <a href="#helm_deps.chart-repository">repository</a>, <a href="#helm_deps.chart-urls">urls</a>, <a href="#helm_deps.chart-version">version</a>)
</pre>

Expose HTTP and OCI Helm charts as Bazel repositories. from_file dependencies are pinned per repository, chart, and version in MODULE.bazel.lock facts; chart tags supply explicit pins.


**TAG CLASSES**

<a id="helm_deps.from_file"></a>

### from_file

Read exact chart dependencies from an ordinary Chart.yaml, resolving only entries missing from Bazel facts.

**Attributes**

| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="helm_deps.from_file-chart"></a>chart |  Source Chart.yaml containing Helm dependency declarations.   | <a href="https://bazel.build/concepts/labels">Label</a> | required |  |

<a id="helm_deps.chart"></a>

### chart

**Attributes**

| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="helm_deps.chart-name"></a>name |  Explicit Bazel repository name for the chart.   | <a href="https://bazel.build/concepts/labels#target-names">Name</a> | required |  |
| <a id="helm_deps.chart-chart"></a>chart |  Published chart name.   | String | required |  |
| <a id="helm_deps.chart-digest"></a>digest |  Chart archive SHA-256, prefixed with sha256:.   | String | required |  |
| <a id="helm_deps.chart-repository"></a>repository |  OCI repository URL without the chart name; mutually exclusive with urls.   | String | optional |  `""`  |
| <a id="helm_deps.chart-urls"></a>urls |  HTTP(S) URLs of the chart archive; mutually exclusive with repository.   | List of strings | optional |  `[]`  |
| <a id="helm_deps.chart-version"></a>version |  Exact chart version.   | String | required |  |


