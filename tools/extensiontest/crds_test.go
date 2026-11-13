package extensiontest

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeChartArchive(t *testing.T, path string, files map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	archive := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		content := files[name]
		if err := archive.WriteHeader(&tar.Header{Name: "example/" + name, Mode: 0600, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHelmShowCRDs(t *testing.T) {
	if *resolverFlag == "" {
		t.Skip("run with -resolver to enable nested Bazel integration tests")
	}
	root, err := filepath.Abs(*rulesRootFlag)
	if err != nil {
		t.Fatal(err)
	}
	f := bazelFixture(t)
	f.write("MODULE.bazel", fmt.Sprintf(`module(name = "helm_crds_test")
bazel_dep(name = "rules_helm", version = "0.5.0")
local_path_override(module_name = "rules_helm", path = %q)
`, root))
	f.write("BUILD.bazel", `load("@rules_helm//:helm.bzl", "helm_chart", "helm_show_crds")

helm_chart(name = "chart", src = "chart with spaces.tgz", version = "1.2.3")
helm_chart(name = "empty_chart", src = "empty.tgz", version = "1.2.3")
helm_show_crds(name = "default_crds", chart = ":chart")
helm_show_crds(name = "custom_crds", chart = ":chart", out = "extracted crds.yaml")
helm_show_crds(name = "empty_crds", chart = ":empty_chart")
helm_show_crds(name = "help", chart = ":chart", arguments = ["--help"])
helm_show_crds(name = "invalid_chart", chart = "chart with spaces.tgz")
`)
	const crd = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.com
  annotations:
    raw: "{{ .Values.notRendered }}"
spec:
  group: example.com
`
	const chart = "apiVersion: v2\nname: example\nversion: 1.2.3\n"
	writeChartArchive(t, filepath.Join(f.workspace, "chart with spaces.tgz"), map[string]string{
		"Chart.yaml":               chart,
		"crds/widgets.yaml":        crd,
		"templates/configmap.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: not-a-crd\n",
	})
	writeChartArchive(t, filepath.Join(f.workspace, "empty.tgz"), map[string]string{"Chart.yaml": chart})
	if _, err := f.command("build", "//:default_crds", "//:custom_crds", "//:empty_crds", "//:help"); err != nil {
		t.Fatalf("helm_show_crds build failed: %v", err)
	}
	readOutput := func(name string) string {
		t.Helper()
		content, err := os.ReadFile(filepath.Join(f.workspace, "bazel-bin", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(content)
	}
	for _, name := range []string{"default_crds.crds.yaml", "extracted crds.yaml"} {
		content := readOutput(name)
		if !strings.Contains(content, crd) || strings.Contains(content, "ConfigMap") {
			t.Fatalf("%s did not contain only unrendered CRDs:\n%s", name, content)
		}
	}
	if content := readOutput("empty_crds.crds.yaml"); content != "" {
		t.Fatalf("chart without CRDs produced nonempty output: %q", content)
	}
	if content := readOutput("help.crds.yaml"); !strings.Contains(content, "Usage:") {
		t.Fatalf("additional arguments were not passed to Helm: %q", content)
	}
	output, err := f.command("build", "//:invalid_chart")
	if err == nil || !strings.Contains(string(output), "HelmChartInfo") {
		t.Fatalf("chart without HelmChartInfo was not rejected: err = %v\n%s", err, output)
	}
}
