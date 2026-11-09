package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestResolveHTTPRepository(t *testing.T) {
	chart := Chart{
		Dependencies: []Dependency{{
			Name:       "cert-manager",
			Version:    "v1.12.0",
			Repository: "https://charts.jetstack.io",
		}},
	}

	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://charts.jetstack.io/index.yaml" {
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`
apiVersion: v1
entries:
  cert-manager:
    - version: v1.12.0
      digest: 1435a289262218111685cf311eb6b57f0e8d788abd4e0b513029043df68c17ae
      urls:
        - charts/cert-manager-v1.12.0.tgz
`)),
		}, nil
	})}

	lock, err := chart.Resolve(context.Background(), client)
	if err != nil {
		t.Fatalf("GenerateLockFile() error = %v", err)
	}

	entry, ok := lock.Entries[resolutionKey("https://charts.jetstack.io", "cert-manager", "v1.12.0")]
	if !ok {
		t.Fatalf("missing resolution: %#v", lock.Entries)
	}

	if entry.Digest != "sha256:1435a289262218111685cf311eb6b57f0e8d788abd4e0b513029043df68c17ae" {
		t.Fatalf("unexpected digest: %s", entry.Digest)
	}

	if entry.Repository != "https://charts.jetstack.io" {
		t.Fatalf("unexpected repository in HTTP entry: %s", entry.Repository)
	}

	if len(entry.Urls) != 1 || entry.Urls[0] != "https://charts.jetstack.io/charts/cert-manager-v1.12.0.tgz" {
		t.Fatalf("unexpected urls: %#v", entry.Urls)
	}
}

func TestNormalizeDigest(t *testing.T) {
	if got := normalizeDigest("abc123"); got != "sha256:abc123" {
		t.Fatalf("normalizeDigest() = %q", got)
	}

	if got := normalizeDigest("sha256:abc123"); got != "sha256:abc123" {
		t.Fatalf("normalizeDigest() preserved digest incorrectly: %q", got)
	}
}

func TestInspectPreservesSourceAndAcceptsYAML(t *testing.T) {
	source := "# Renovate owns versions\ndependencies:\n  - name: example\n    version: '1.2.3' # retain this comment\n    repository: https://example.com/charts\n"
	path := filepath.Join(t.TempDir(), "Chart.yaml")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"inspect", path}, &output); err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Protocol != 1 || len(response.Dependencies) != 1 || response.Dependencies[0].Version != "1.2.3" {
		t.Fatalf("unexpected response: %+v", response)
	}
	after, _ := os.ReadFile(path)
	if string(after) != source {
		t.Fatal("inspect changed Chart.yaml")
	}
}

func TestValidateDependencies(t *testing.T) {
	for _, version := range []string{"", "^1.0.0", "1.x", ">=1.0.0", "latest"} {
		if err := validateDependencies([]Dependency{{Name: "chart", Version: version, Repository: "https://example.com"}}); err == nil {
			t.Errorf("accepted non-exact version %q", version)
		}
	}
	for _, version := range []string{"1.2.3", "v1.2.3", "1.2.3-exp.1+build"} {
		if err := validateDependencies([]Dependency{{Name: "chart", Version: version, Repository: "oci://example.com/charts"}}); err != nil {
			t.Error(err)
		}
	}
	for _, repository := range []string{"@stable", "file://../chart", "https://user:secret@example.com", "https://example.com?token=secret"} {
		if err := validateDependencies([]Dependency{{Name: "chart", Version: "1.2.3", Repository: repository}}); err == nil {
			t.Errorf("accepted repository %q", repository)
		}
	}
}

func TestResolveProtocolBatchesAndDeduplicates(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/charts/index.yaml" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		fmt.Fprintf(w, "entries:\n  example:\n    - version: 1.2.3\n      digest: %s\n      urls: [../example.tgz]\n", strings.Repeat("a", 64))
	}))
	defer server.Close()
	dep := Dependency{Name: "example", Version: "1.2.3", Repository: server.URL + "/charts"}
	request := Request{Protocol: 1, Dependencies: []Dependency{dep, dep}}
	input, _ := json.Marshal(request)
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, input, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{"resolve", path}, &output); err != nil {
		t.Fatal(err)
	}
	var response Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || len(response.Resolutions) != 2 || response.Resolutions[0].Urls[0] != server.URL+"/example.tgz" {
		t.Fatalf("requests=%d, response=%+v", requests, response)
	}
}

func TestResolveRejectsBadIndexes(t *testing.T) {
	for _, tc := range []struct {
		name, index string
		status      int
	}{
		{"status", "", 404},
		{"digest", "entries: {example: [{version: 1.2.3, digest: bad, urls: [chart.tgz]}]}", 200},
		{"missing", "entries: {}", 200},
		{"urls", "entries: {example: [{version: 1.2.3, digest: " + strings.Repeat("a", 64) + "}]}", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.index)
			}))
			defer server.Close()
			chart := Chart{Dependencies: []Dependency{{Name: "example", Version: "1.2.3", Repository: server.URL}}}
			if _, err := chart.Resolve(context.Background(), server.Client()); err == nil {
				t.Fatal("accepted invalid index")
			}
		})
	}
}

func TestProtocolRejectsUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, []byte(`{"protocol":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"resolve", path}, io.Discard); err == nil {
		t.Fatal("accepted unknown protocol")
	}
}

func TestResolveOCIRecordsVersionAndArchiveDigest(t *testing.T) {
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	layer := static.NewLayer([]byte("helm chart archive"), types.MediaType(helmChartLayerMediaType))
	image, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := name.NewTag(host+"/helm/example:1.2.3", name.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Write(ref, image, remote.WithAuth(authn.Anonymous)); err != nil {
		t.Fatal(err)
	}
	repository := "oci://" + host + "/helm"
	chart := Chart{Dependencies: []Dependency{{Name: "example", Version: "1.2.3", Repository: repository}}}
	resolved, err := chart.Resolve(context.Background(), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	wantDigest, _ := layer.Digest()
	entry := resolved.Entries[resolutionKey(repository, "example", "1.2.3")]
	if entry.Repository != repository || entry.Chart != "example" || entry.Version != "1.2.3" || entry.Digest != wantDigest.String() || len(entry.Urls) != 0 {
		t.Fatalf("unexpected OCI resolution: %+v", entry)
	}
	index := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: image})
	gotDigest, err := resolveOCIChartDigestFromIndex(index)
	if err != nil || gotDigest != entry.Digest {
		t.Fatalf("OCI index digest = %s, error = %v", gotDigest, err)
	}
}
