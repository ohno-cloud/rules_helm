package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"gopkg.in/yaml.v3"
)

// A subset of the Helm Chart.yaml file.
// https://helm.sh/docs/topics/charts
type Chart struct {
	// A list of the chart requirements.
	Dependencies []Dependency `yaml:"dependencies" json:"dependencies"`
}

type Dependency struct {
	// The name of the chart (nginx)
	Name string `yaml:"name" json:"name"`
	// The version of the chart ("1.2.3")
	Version string `yaml:"version" json:"version"`
	// An HTTP(S) or OCI repository URL; aliases are not supported.
	Repository string `yaml:"repository" json:"repository"`
}

type resolutionSet struct {
	Entries map[string]Resolution
}

type Resolution struct {
	Digest     string   `yaml:"digest" json:"digest"`
	Urls       []string `yaml:"urls,omitempty" json:"urls,omitempty"`
	Repository string   `yaml:"repository,omitempty" json:"repository,omitempty"`
	Chart      string   `yaml:"chart" json:"chart"`
	Version    string   `yaml:"version" json:"version"`
}

type RepositoryIndex struct {
	ApiVersion string `yaml:"apiVersion"`
	Entries    map[string][]struct {
		ApiVersion string   `yaml:"apiVersion"`
		Name       string   `yaml:"name"`
		Urls       []string `yaml:"urls"`
		Version    string   `yaml:"version"`
		Digest     string   `yaml:"digest"`
	} `yaml:"entries"`
}

const helmChartLayerMediaType = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

const protocolVersion = 1

// Request and Response form the extension-time protocol, not a source lockfile.
type Request struct {
	Protocol     int          `json:"protocol"`
	Dependencies []Dependency `json:"dependencies"`
}

type Response struct {
	Protocol     int          `json:"protocol"`
	Dependencies []Dependency `json:"dependencies,omitempty"`
	Resolutions  []Resolution `json:"resolutions,omitempty"`
}

func run(args []string, out io.Writer) error {
	if len(args) != 2 || (args[0] != "inspect" && args[0] != "resolve") {
		return fmt.Errorf("usage: helmlock inspect <Chart.yaml> | helmlock resolve <request.json>")
	}
	buf, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	response := Response{Protocol: protocolVersion}
	if args[0] == "inspect" {
		var chart Chart
		if err := yaml.Unmarshal(buf, &chart); err != nil {
			return fmt.Errorf("parse Chart.yaml: %w", err)
		}
		if err := validateDependencies(chart.Dependencies); err != nil {
			return err
		}
		response.Dependencies = chart.Dependencies
	} else {
		var request Request
		if err := json.Unmarshal(buf, &request); err != nil {
			return err
		}
		if request.Protocol != protocolVersion {
			return fmt.Errorf("unsupported protocol %d, want %d", request.Protocol, protocolVersion)
		}
		if err := validateDependencies(request.Dependencies); err != nil {
			return err
		}
		chart := Chart{Dependencies: request.Dependencies}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		resolved, err := chart.Resolve(ctx, &http.Client{Timeout: 60 * time.Second})
		if err != nil {
			return err
		}
		// Return request order, independent of Go map iteration and Bazel names.
		for _, dep := range request.Dependencies {
			entry, ok := resolved.Entries[resolutionKey(dep.Repository, dep.Name, dep.Version)]
			if !ok {
				return fmt.Errorf("missing resolution for %s@%s", dep.Name, dep.Version)
			}
			response.Resolutions = append(response.Resolutions, entry)
		}
	}
	return json.NewEncoder(out).Encode(response)
}

var exactVersion = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

func validateDependencies(deps []Dependency) error {
	for _, dep := range deps {
		if dep.Name == "" || strings.ContainsAny(dep.Name, "/@:") {
			return fmt.Errorf("invalid chart name %q", dep.Name)
		}
		if !exactVersion.MatchString(dep.Version) {
			return fmt.Errorf("chart %s requires an exact version, got %q", dep.Name, dep.Version)
		}
		u, err := url.Parse(dep.Repository)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "oci") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("chart %s has an unsupported repository: use an http(s) or oci URL without credentials, query, or fragment", dep.Name)
		}
	}
	return nil
}

// groupByRepository groups the Dependencies field group repository, then
// by charts and by wanted versions.
func (c *Chart) groupByRepository() map[string]map[string][]string {
	output := make(map[string]map[string][]string)
	for _, dep := range c.Dependencies {
		if repo, ok := output[dep.Repository]; ok {
			if chart, ok := repo[dep.Name]; ok {
				if !slices.Contains(chart, dep.Version) {
					chart = append(chart, dep.Version)
				}
				repo[dep.Name] = chart
			} else {
				repo[dep.Name] = []string{dep.Version}
			}
			output[dep.Repository] = repo
		} else {
			val := map[string][]string{
				dep.Name: {dep.Version},
			}
			output[dep.Repository] = val
		}
	}

	return output
}

func (c *Chart) Resolve(ctx context.Context, client *http.Client) (*resolutionSet, error) {
	lock := &resolutionSet{
		Entries: make(map[string]Resolution),
	}

	wants := c.groupByRepository()

	for repo, wantCharts := range wants {
		var err error
		if isOCIRepository(repo) {
			err = generateOCILockEntries(ctx, lock, repo, wantCharts)
		} else {
			err = generateHTTPLockEntries(ctx, client, lock, repo, wantCharts)
		}
		if err != nil {
			return nil, err
		}
	}

	return lock, nil
}

func generateHTTPLockEntries(ctx context.Context, client *http.Client, lock *resolutionSet, repo string, wantCharts map[string][]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/index.yaml", strings.TrimRight(repo, "/")), nil)
	if err != nil {
		return fmt.Errorf("failed request from repo %s: %w", repo, err)
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch index for %s: HTTP %s", repo, res.Status)
	}

	buf, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	var repoIndex RepositoryIndex
	if err = yaml.Unmarshal(buf, &repoIndex); err != nil {
		return err
	}

	foundCharts := map[string]bool{}

	for chart, chartVersions := range repoIndex.Entries {
		foundVersions := map[string]bool{}
		seenVersions := []string{}

		if wantVersions, ok := wantCharts[chart]; ok {
			foundCharts[chart] = true
			for _, version := range chartVersions {
				seenVersions = append(seenVersions, version.Version)
				if slices.Contains(wantVersions, version.Version) {
					if foundVersions[version.Version] {
						return fmt.Errorf("duplicate version %s of chart %s in %s", version.Version, chart, repo)
					}
					digest := normalizeDigest(version.Digest)
					decoded, err := hex.DecodeString(strings.TrimPrefix(digest, "sha256:"))
					if err != nil || len(decoded) != 32 || !strings.HasPrefix(digest, "sha256:") {
						return fmt.Errorf("invalid SHA-256 for %s@%s in %s", chart, version.Version, repo)
					}
					urls, err := checkUrls(repo, version.Urls)
					if err != nil {
						return err
					}
					lockName := resolutionKey(repo, chart, version.Version)
					lock.Entries[lockName] = Resolution{
						Repository: repo,
						Chart:      chart,
						Version:    version.Version,
						Digest:     digest,
						Urls:       urls,
					}

					foundVersions[version.Version] = true
				}
			}

			if len(foundVersions) != len(wantVersions) {
				return fmt.Errorf("did not find all versions of chart %s, found %v and saw %v", chart, foundVersions, seenVersions)
			}
		}
	}

	if len(foundCharts) != len(wantCharts) {
		return fmt.Errorf("did not find all charts for repository %s, found %v", repo, foundCharts)
	}

	return nil
}

func generateOCILockEntries(ctx context.Context, lock *resolutionSet, repo string, wantCharts map[string][]string) error {
	for chart, wantVersions := range wantCharts {
		for _, version := range wantVersions {
			digest, err := resolveOCIChartDigest(ctx, repo, chart, version)
			if err != nil {
				return err
			}

			lockName := resolutionKey(repo, chart, version)
			lock.Entries[lockName] = Resolution{
				Chart:      chart,
				Version:    version,
				Digest:     digest,
				Repository: repo,
			}
		}
	}

	return nil
}

func resolveOCIChartDigest(ctx context.Context, repo, chart, version string) (string, error) {
	ref, err := name.ParseReference(fmt.Sprintf("%s/%s:%s", strings.TrimSuffix(strings.TrimPrefix(repo, "oci://"), "/"), chart, strings.ReplaceAll(version, "+", "_")))
	if err != nil {
		return "", fmt.Errorf("parse oci reference for %s/%s:%s: %w", repo, chart, version, err)
	}

	desc, err := remote.Get(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		return "", fmt.Errorf("resolve oci reference %s: %w", ref.Name(), err)
	}

	manifest, err := desc.ImageIndex()
	if err == nil {
		return resolveOCIChartDigestFromIndex(manifest)
	}

	image, err := desc.Image()
	if err != nil {
		return "", fmt.Errorf("read oci manifest for %s: %w", ref.Name(), err)
	}

	return resolveOCIChartDigestFromImage(image)
}

func resolveOCIChartDigestFromIndex(index v1.ImageIndex) (string, error) {
	indexManifest, err := index.IndexManifest()
	if err != nil {
		return "", err
	}

	for _, manifest := range indexManifest.Manifests {
		image, err := index.Image(manifest.Digest)
		if err != nil {
			return "", err
		}

		digest, err := resolveOCIChartDigestFromImage(image)
		if err == nil {
			return digest, nil
		}
	}

	return "", fmt.Errorf("did not find a helm chart layer in OCI index")
}

func resolveOCIChartDigestFromImage(image v1.Image) (string, error) {
	manifest, err := image.Manifest()
	if err != nil {
		return "", err
	}

	for _, layer := range manifest.Layers {
		if string(layer.MediaType) == helmChartLayerMediaType {
			return layer.Digest.String(), nil
		}
	}

	return "", fmt.Errorf("did not find a helm chart layer in OCI manifest")
}

func resolutionKey(repo, chart, version string) string {
	key, _ := json.Marshal([]string{repo, chart, version})
	return string(key)
}

func isOCIRepository(repo string) bool {
	return strings.HasPrefix(strings.ToLower(repo), "oci://")
}

func normalizeDigest(digest string) string {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return digest
	}
	if strings.Contains(digest, ":") {
		return digest
	}
	return "sha256:" + digest
}

func checkUrls(repo string, urls []string) ([]string, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("no chart archive URLs in %s", repo)
	}
	base, err := url.Parse(strings.TrimRight(repo, "/") + "/")
	if err != nil {
		return nil, err
	}
	checked := make([]string, len(urls))
	for idx, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, err
		}
		u = base.ResolveReference(u)
		if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return nil, fmt.Errorf("unsupported chart archive URL %q", raw)
		}
		checked[idx] = u.String()
	}
	return checked, nil
}
