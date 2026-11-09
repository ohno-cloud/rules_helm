// Package extensiontest exercises the real Bazel extension with a local repository.
package extensiontest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var resolverFlag = flag.String("resolver", "", "absolute path to helmlock; enables the Bazel integration test")
var rulesRootFlag = flag.String("rules-root", "../..", "rules_helm source directory for the nested Bazel workspace")

type resolution struct {
	Repository string   `json:"repository"`
	Chart      string   `json:"chart"`
	Version    string   `json:"version"`
	Digest     string   `json:"digest"`
	URLs       []string `json:"urls"`
}

type fixture struct {
	t         *testing.T
	workspace string
	resolver  string
	module    string
	repo      string
	indexes   atomic.Int64
	offline   atomic.Bool
	tampered  atomic.Bool
}

func (f *fixture) write(name, content string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.workspace, name), []byte(content), 0600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) read(name string) []byte {
	f.t.Helper()
	content, err := os.ReadFile(filepath.Join(f.workspace, name))
	if err != nil {
		f.t.Fatal(err)
	}
	return content
}

func (f *fixture) command(args ...string) ([]byte, error) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	args = append([]string{"--output_user_root=" + filepath.Join(filepath.Dir(f.workspace), "bazel-root")}, args...)
	cmd := exec.CommandContext(ctx, "bazel", args...)
	cmd.Dir = f.workspace
	output, err := cmd.CombinedOutput()
	f.t.Logf("bazel %s:\n%s", strings.Join(args, " "), output)
	return output, err
}

func (f *fixture) bazel(fail bool, command string, extra ...string) string {
	f.t.Helper()
	args := []string{command, "--repo_env=RULES_HELM_RESOLVER=" + f.resolver,
		"--repository_cache=" + filepath.Join(filepath.Dir(f.workspace), "repository-cache")}
	output, err := f.command(append(args, extra...)...)
	if fail && err == nil {
		f.t.Fatal("expected Bazel failure")
	}
	if !fail && err != nil {
		f.t.Fatalf("Bazel integration test failed: %v", err)
	}
	return string(output)
}

func (f *fixture) chart(versions ...string) string {
	f.t.Helper()
	unique := slices.Clone(versions)
	slices.Sort(unique)
	unique = slices.Compact(unique)
	names := []string{strconv.Quote("1_0_0_127_example")}
	if len(unique) > 1 {
		names = nil
		for _, version := range unique {
			names = append(names, strconv.Quote("1_0_0_127_example_"+strings.ReplaceAll(version, ".", "_")))
		}
	}
	f.write("MODULE.bazel", f.module+"use_repo(helm, "+strings.Join(names, ", ")+")\n")
	var chart strings.Builder
	chart.WriteString("# Keep YAML comments and quoting\ndependencies:\n")
	for _, version := range versions {
		fmt.Fprintf(&chart, "  - name: example\n    version: '%s'\n    repository: %s\n", version, f.repo)
	}
	f.write("Chart.yaml", chart.String())
	return chart.String()
}

func (f *fixture) facts() map[string]resolution {
	f.t.Helper()
	var lock struct {
		Facts            map[string]map[string]json.RawMessage `json:"facts"`
		ModuleExtensions map[string]json.RawMessage            `json:"moduleExtensions"`
	}
	if err := json.Unmarshal(f.read("MODULE.bazel.lock"), &lock); err != nil {
		f.t.Fatal(err)
	}
	for key := range lock.ModuleExtensions {
		if strings.HasSuffix(key, "%helm_deps") {
			f.t.Fatal("extension is not marked reproducible")
		}
	}
	var result map[string]resolution
	for key, entries := range lock.Facts {
		if !strings.HasSuffix(key, "%helm_deps") {
			continue
		}
		if result != nil {
			f.t.Fatal("multiple Helm extension fact sets")
		}
		result = make(map[string]resolution)
		for key, content := range entries {
			var value resolution
			if err := json.Unmarshal(content, &value); err != nil {
				f.t.Fatal(err)
			}
			result[key] = value
		}
	}
	if result == nil {
		f.t.Fatal("no Helm extension facts found")
	}
	return result
}

func TestFactsBackedExtension(t *testing.T) {
	if *resolverFlag == "" {
		t.Skip("run with -resolver /absolute/path/to/helmlock to enable the Bazel integration test")
	}
	resolver, err := filepath.Abs(*resolverFlag)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err = filepath.EvalSymlinks(resolver)
	if err != nil {
		t.Fatalf("resolver executable is unavailable: %v", err)
	}
	root, err := filepath.Abs(*rulesRootFlag)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, workspace: filepath.Join(t.TempDir(), "workspace"), resolver: resolver}
	if err := os.Mkdir(f.workspace, 0700); err != nil {
		t.Fatal(err)
	}
	archive := []byte("test Helm chart archive")
	digest := fmt.Sprintf("%x", sha256.Sum256(archive))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/charts/index.yaml":
			f.indexes.Add(1)
			if f.offline.Load() {
				http.Error(w, "offline", http.StatusServiceUnavailable)
				return
			}
			entries := []map[string]any{}
			for _, version := range []string{"1.2.3", "2.0.0"} {
				entries = append(entries, map[string]any{"version": version, "digest": digest, "urls": []string{"example.tgz"}})
			}
			if err := json.NewEncoder(w).Encode(map[string]any{"entries": map[string]any{"example": entries}}); err != nil {
				t.Error(err)
			}
		case "/charts/example.tgz":
			body := archive
			if f.tampered.Load() {
				body = []byte("tampered archive")
			}
			if _, err := w.Write(body); err != nil {
				t.Error(err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() {
		if _, err := f.command("shutdown"); err != nil {
			t.Errorf("Bazel shutdown: %v", err)
		}
	})
	f.repo = server.URL + "/charts"
	f.module = "module(name = \"helm_extension_test\")\n" +
		"bazel_dep(name = \"rules_helm\", version = \"0.4.0\")\n" +
		fmt.Sprintf("local_path_override(module_name = \"rules_helm\", path = %q)\n", root) +
		"helm = use_extension(\"@rules_helm//:extensions.bzl\", \"helm_deps\")\n" +
		"helm.from_file(chart = \"//:Chart.yaml\")\n"
	f.write("BUILD.bazel", "exports_files([\"Chart.yaml\"])\n")
	original := f.chart("1.2.3")
	f.bazel(false, "mod", "deps")
	first := f.facts()
	if len(first) != 1 || f.indexes.Load() != 1 {
		t.Fatalf("initial facts = %v, index requests = %d", first, f.indexes.Load())
	}
	for _, value := range first {
		if value.Digest != "sha256:"+digest || value.Repository != f.repo || value.Chart != "example" || value.Version != "1.2.3" {
			t.Fatalf("unexpected fact: %+v", value)
		}
	}
	if string(f.read("Chart.yaml")) != original {
		t.Fatal("extension modified Chart.yaml")
	}

	// Changed watched input forces reevaluation, without resolving known versions.
	f.offline.Store(true)
	f.write("Chart.yaml", original+"# force reevaluation\n")
	f.bazel(false, "mod", "deps", "--lockfile_mode=error")
	if f.indexes.Load() != 1 {
		t.Fatal("warm facts caused a repository query")
	}
	f.tampered.Store(true)
	output := f.bazel(true, "fetch", "@1_0_0_127_example//:chart")
	if !strings.Contains(strings.ToLower(output), "checksum") {
		t.Fatal("tampering did not cause a checksum error")
	}

	f.offline.Store(false)
	f.tampered.Store(false)
	f.chart("1.2.3", "2.0.0", "1.2.3")
	f.bazel(false, "mod", "deps")
	second := f.facts()
	if len(second) != 2 || f.indexes.Load() != 2 {
		t.Fatalf("updated facts = %v, index requests = %d", second, f.indexes.Load())
	}
	for key, value := range first {
		if !reflect.DeepEqual(second[key], value) {
			t.Fatal("existing resolution changed during a version update")
		}
	}

	// Reuse the committed facts in a fresh Bazel server, with resolution offline.
	if _, err := f.command("shutdown"); err != nil {
		t.Fatal(err)
	}
	f.offline.Store(true)
	f.bazel(false, "mod", "deps", "--lockfile_mode=error")
	if f.indexes.Load() != 2 {
		t.Fatal("server restart caused a repository query")
	}

	// Removing the second version restores the original unsuffixed repository.
	f.chart("1.2.3")
	f.bazel(false, "mod", "deps")
	f.bazel(false, "fetch", "@1_0_0_127_example//:chart")
	if f.indexes.Load() != 2 {
		t.Fatal("removing a dependency caused a repository query")
	}

	// A manual declaration must never silently overwrite a file dependency.
	f.write("MODULE.bazel", string(f.read("MODULE.bazel"))+
		`helm.chart(name = "1_0_0_127_example", chart = "example", version = "1.2.3", digest = "sha256:`+
		strings.Repeat("b", 64)+`", urls = ["https://example.com/chart.tgz"])`+"\n")
	output = f.bazel(true, "mod", "deps")
	if !strings.Contains(output, "conflicting Helm dependencies") {
		t.Fatal("conflicting declaration did not cause an error")
	}
}
