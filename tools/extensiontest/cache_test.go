package extensiontest

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Serve the HTTP remote-cache protocol so this test doesn't need an RBE service.
// Separate workspaces and disabled local caches ensure reuse really is remote.
func repositoryCache(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var mu sync.Mutex
	entries := map[string][]byte{}
	hits := &atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			entries[r.URL.Path] = body
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			mu.Lock()
			body, ok := entries[r.URL.Path]
			mu.Unlock()
			if !ok {
				http.NotFound(w, r)
				return
			}
			hits.Add(1)
			_, _ = w.Write(body)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)
	return server, hits
}

func bazelFixture(t *testing.T, startup ...string) *fixture {
	t.Helper()
	f := &fixture{
		t:         t,
		workspace: filepath.Join(t.TempDir(), "workspace"),
		startup:   append([]string{"--ignore_all_rc_files"}, startup...),
	}
	if err := os.Mkdir(f.workspace, 0700); err != nil {
		t.Fatal(err)
	}
	f.write("BUILD.bazel", "")
	t.Cleanup(func() {
		if _, err := f.command("shutdown"); err != nil {
			t.Errorf("Bazel shutdown: %v", err)
		}
	})
	return f
}

func requireHTTPRepositoryCache(t *testing.T) {
	t.Helper()
	if *resolverFlag == "" {
		t.Skip("run with -resolver to enable nested Bazel integration tests")
	}
	cmd := exec.Command("bazel", "--version")
	cmd.Dir = t.TempDir()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Bazel version: %v\n%s", err, output)
	}
	if strings.HasPrefix(strings.TrimSpace(string(output)), "bazel 9.1.") {
		t.Skip("Bazel 9.1 only initializes repository caching with gRPC; HTTP fixtures require Bazel 9.2+")
	}
}

func TestRemoteRepositoryCache(t *testing.T) {
	requireHTTPRepositoryCache(t)
	root, err := filepath.Abs(*rulesRootFlag)
	if err != nil {
		t.Fatal(err)
	}
	cache, hits := repositoryCache(t)
	archive := []byte("pinned Helm chart archive")
	digest := fmt.Sprintf("%x", sha256.Sum256(archive))
	var requests, tokens atomic.Int64
	var offline atomic.Bool
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if offline.Load() {
			http.Error(w, "origin must not be needed on a cache hit", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/example.tgz", "/v2/charts/example/blobs/sha256:" + digest:
			_, _ = w.Write(archive)
		case "/v2/charts/example/manifests/1.2.3":
			fmt.Fprintf(w, `{"layers":[{"mediaType":"application/vnd.cncf.helm.chart.content.v1.tar+gzip","digest":"sha256:%s"}]}`, digest)
		case "/token":
			// Vary the token on each request: cached repo bytes must not depend on it.
			fmt.Fprintf(w, `{"token":"secret-token-%d"}`, tokens.Add(1))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(origin.Close)

	// Redirect the real OCI downloader to a local fixture without requiring TLS or DNS.
	configDir := t.TempDir()
	downloader := filepath.Join(configDir, "downloader.cfg")
	if err := os.WriteFile(downloader, []byte("rewrite registry.rules-helm.test/(.*) "+origin.URL+"/$1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	authFile := filepath.Join(configDir, "auth.json")
	// Initially absent: adding a higher-priority auth file must invalidate the repo.
	module := fmt.Sprintf(`module(name = "helm_cache_test")
bazel_dep(name = "rules_helm", version = "0.5.0")
local_path_override(module_name = "rules_helm", path = %q)
chart = use_repo_rule("@rules_helm//:rules.bzl", "helm_repo_chart")
chart(name = "http_chart", chart = "example", version = "1.2.3", sha256 = %q, urls = [%q])
chart(
    name = "oci_chart",
    chart = "example",
    version = "1.2.3",
    sha256 = "sha256:%s",
    repository = "oci://registry.rules-helm.test/charts",
    www_authenticate_challenges = {"registry.rules-helm.test": 'Bearer realm="https://registry.rules-helm.test/token",service="test"'},
)
`, root, digest, origin.URL+"/example.tgz", digest)
	options := []string{
		"--repository_cache=", "--repo_contents_cache=",
		"--remote_cache=" + cache.URL,
		"--downloader_config=" + downloader,
		"--repo_env=REGISTRY_AUTH_FILE=" + authFile,
		"--repo_env=HOME=" + configDir,
		"--repo_env=DOCKER_CONFIG=" + configDir,
		"--repo_env=XDG_RUNTIME_DIR=",
	}
	run := func(f *fixture, fail bool, targets ...string) string {
		t.Helper()
		args := append([]string{"build"}, options...)
		output, err := f.command(append(args, targets...)...)
		if (err != nil) != fail {
			t.Fatalf("Bazel failure = %v, want failure = %v", err, fail)
		}
		return string(output)
	}

	warm := bazelFixture(t, "--experimental_remote_repo_contents_cache")
	warm.write("MODULE.bazel", module)
	run(warm, false, "@http_chart//:chart", "@oci_chart//:chart")
	if tokens.Load() != 1 {
		t.Fatalf("token requests = %d, want 1", tokens.Load())
	}
	// Inspect the locally evaluated OCI repo before the cache can inject it lazily.
	repos, err := filepath.Glob(filepath.Join(filepath.Dir(warm.workspace), "bazel-root", "*", "external", "*+oci_chart"))
	if err != nil || len(repos) != 1 {
		t.Fatalf("find OCI repo: paths = %v, err = %v", repos, err)
	}
	files, err := os.ReadDir(repos[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Name() != "BUILD.bazel" && file.Name() != "example-1.2.3.tgz" && file.Name() != "REPO.bazel" {
			t.Errorf("unexpected cached repository file: %s", file.Name())
		}
	}

	before, cacheHits := requests.Load(), hits.Load()
	offline.Store(true)
	client := bazelFixture(t, "--experimental_remote_repo_contents_cache")
	client.write("MODULE.bazel", module)
	run(client, false, "@http_chart//:chart", "@oci_chart//:chart")
	if requests.Load() != before || hits.Load() <= cacheHits {
		t.Fatalf("remote reuse failed: origin requests %d -> %d, cache hits %d -> %d", before, requests.Load(), cacheHits, hits.Load())
	}

	// Creating previously absent credentials must refetch the OCI repo, not reuse
	// the old cache entry. Auth responses must still not become repository outputs.
	offline.Store(false)
	if err := os.WriteFile(authFile, []byte(`{"auths":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	run(client, false, "@oci_chart//:chart")
	if tokens.Load() != 2 {
		t.Fatalf("adding auth file did not invalidate OCI repo: token requests = %d", tokens.Load())
	}

	// A changed digest must miss the old entry and verify the downloaded bytes.
	client.write("MODULE.bazel", strings.Replace(module, digest, strings.Repeat("b", 64), 1))
	output := run(client, true, "@http_chart//:chart")
	if !strings.Contains(strings.ToLower(output), "checksum") {
		t.Fatal("changed digest did not cause a checksum error")
	}

	for _, invalid := range []string{"", "sha256:", "abcd", strings.Repeat("g", 64)} {
		client.write("MODULE.bazel", strings.Replace(module, digest, invalid, 1))
		before := requests.Load()
		output := run(client, true, "@http_chart//:chart")
		if !strings.Contains(output, "helm_repo_chart sha256 must be a nonempty SHA-256 digest") {
			t.Fatalf("invalid digest %q was not rejected", invalid)
		}
		if requests.Load() != before {
			t.Fatalf("invalid digest %q caused an origin request", invalid)
		}
	}
}

func TestResolverRepositoryCache(t *testing.T) {
	requireHTTPRepositoryCache(t)
	source, err := os.ReadFile(filepath.Join(*rulesRootFlag, "private", "resolver.bzl"))
	if err != nil {
		t.Fatal(err)
	}
	cache, hits := repositoryCache(t)
	binary := []byte("pinned resolver executable")
	digest := fmt.Sprintf("%x", sha256.Sum256(binary))
	var requests atomic.Int64
	var offline atomic.Bool
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if offline.Load() {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(binary)
	}))
	t.Cleanup(origin.Close)
	downloader := filepath.Join(t.TempDir(), "downloader.cfg")
	if err := os.WriteFile(downloader, []byte("rewrite github.com/ohno-cloud/rules_helm/releases/download/(.*) "+origin.URL+"/$1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	create := func() *fixture {
		t.Helper()
		f := bazelFixture(t, "--experimental_remote_repo_contents_cache")
		// Use the actual rule implementation with fixture release metadata, avoiding
		// a network dependency on a published resolver or any host executable.
		f.write("resolver.bzl", string(source))
		f.write("MODULE.bazel", `module(name = "resolver_cache_test")
resolver = use_repo_rule("//:resolver.bzl", "helmlock_repository")
resolver(name = "resolver")
`)
		if err := os.Mkdir(filepath.Join(f.workspace, "private"), 0700); err != nil {
			t.Fatal(err)
		}
		f.write("private/BUILD.bazel", "exports_files([\"helmlock.json\"])\n")
		f.write("private/helmlock.json", fmt.Sprintf(`{"tag":"fixture","sha256":{%q:%q}}`, runtime.GOOS+"-"+runtime.GOARCH, digest))
		return f
	}
	run := func(f *fixture, override string) {
		t.Helper()
		output, err := f.command("build", "@resolver//:helmlock",
			"--repository_cache=", "--repo_contents_cache=", "--remote_cache="+cache.URL,
			"--downloader_config="+downloader, "--repo_env=RULES_HELM_RESOLVER="+override)
		if err != nil {
			t.Fatalf("resolver fetch failed: %v\n%s", err, output)
		}
	}
	run(create(), "")
	if requests.Load() != 1 {
		t.Fatalf("resolver downloads = %d, want 1", requests.Load())
	}
	cacheHits := hits.Load()
	offline.Store(true)
	run(create(), "")
	if requests.Load() != 1 || hits.Load() <= cacheHits {
		t.Fatal("released resolver was not reused from remote cache")
	}

	// An override must stay a watched symlink, not turn into a remotely cached
	// executable. Rebuilding after editing the override must expose the new bytes.
	override := filepath.Join(t.TempDir(), "helmlock")
	if err := os.WriteFile(override, []byte("local resolver"), 0700); err != nil {
		t.Fatal(err)
	}
	local := create()
	run(local, override)
	if err := os.WriteFile(override, []byte("updated local resolver"), 0700); err != nil {
		t.Fatal(err)
	}
	run(local, override)
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(local.workspace), "bazel-root", "*", "external", "*+resolver", "helmlock"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("find local resolver: paths = %v, err = %v", paths, err)
	}
	if target, err := os.Readlink(paths[0]); err != nil || target != override {
		t.Fatalf("local resolver is not the expected symlink: target = %q, err = %v", target, err)
	}
	content, err := os.ReadFile(paths[0])
	if err != nil || string(content) != "updated local resolver" {
		t.Fatalf("local resolver did not update: content = %q, err = %v", content, err)
	}
}
