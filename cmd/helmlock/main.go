package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// A subset of the Helm Charts.yaml file
// https://helm.sh/docs/topics/charts
type Chart struct {
	//# A list of the chart requirements
	Dependencies []struct {
		// The name of the chart (nginx)
		Name string `yaml:"name" json:"name"`
		// The version of the chart ("1.2.3")
		Version string `yaml:"version" json:"version"`
		// The repository URL ("https://example.com/charts") or alias ("@repo-name")
		Repository string `yaml:"repository" json:"repository"`
	} `yaml:"dependencies" json:"dependencies"`
}

type Lockfile struct {
	// Repositories is a map of bazel repository names to lock file contents
	Repositories map[string]LockEntry `yaml:"repositories" json:"repositories"`
}

type LockEntry struct {
	Digest  string   `yaml:"digest" json:"digest"`
	Urls    []string `yaml:"urls" json:"urls"`
	Chart   string   `yaml:"chart" json:"chart"`
	Version string   `yaml:"version" json:"version"`
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

var inputFile = flag.String("chart", "", "path to Chart.yaml to generate from")
var outputFile = flag.String("output", "", "path to write lock file to")

func main() {
	flag.Parse()

	file, err := os.Open(*inputFile)
	if err != nil {
		fmt.Printf("failed to open file: %s - %s", *inputFile, err)
		os.Exit(1)
	}
	defer file.Close()

	buf, err := io.ReadAll(file)
	if err != nil {
		fmt.Printf("failed read file: %s", err)
		os.Exit(2)
	}

	var chart Chart
	if err = yaml.Unmarshal(buf, &chart); err != nil {
		fmt.Printf("failed unmarshal file: %s", err)
		os.Exit(3)
	}

	lock, err := chart.GenerateLockFile(context.Background(), http.DefaultClient)
	if err != nil {
		fmt.Printf("failed to generate lock file: %s", err)
		fmt.Fprintf(os.Stderr, "failed to generate lock file: %s", err)
		os.Exit(4)
	}

	lockBuf, err := json.Marshal(lock)
	if err != nil {
		fmt.Printf("failed to marshal lock file to json: %s", err)
		os.Exit(5)
	}

	if *outputFile == "" {
		fmt.Printf("%s", lockBuf)
	} else {
		out, err := os.OpenFile(*outputFile, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
		if err != nil {
			fmt.Printf("failed to open output file (%s): %s", *outputFile, err)
			os.Exit(6)
		}
		defer out.Close()

		if _, err = out.Write(lockBuf); err != nil {
			fmt.Printf("failed to write to output file (%s): %s", *outputFile, err)
			os.Exit(7)
		}
	}
}

// groupByRepository groups the Dependencies field group repository, then
// by charts and by wanted versions.
func (c *Chart) groupByRepository() map[string]map[string][]string {
	output := make(map[string]map[string][]string)
	for _, dep := range c.Dependencies {
		if repo, ok := output[dep.Repository]; ok {
			if chart, ok := repo[dep.Name]; ok {
				chart = append(chart, dep.Version)
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

func (c *Chart) GenerateLockFile(ctx context.Context, client *http.Client) (*Lockfile, error) {
	var lock Lockfile
	lock.Repositories = make(map[string]LockEntry)

	wants := c.groupByRepository()

	for repo, wantCharts := range wants {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/index.yaml", repo), nil)
		if err != nil {
			return nil, fmt.Errorf("failed request from repo %s: %w", repo, err)
		}

		res, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()

		buf, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}

		var repoIndex RepositoryIndex
		if err = yaml.Unmarshal(buf, &repoIndex); err != nil {
			return nil, err
		}

		foundCharts := map[string]bool{}

		for chart, chartVersions := range repoIndex.Entries {
			foundVersions := map[string]bool{}
			// Filter out chart entries we're not after
			if wantVersions, ok := wantCharts[chart]; ok {
				foundCharts[chart] = true
				for _, version := range chartVersions {

					if slices.Contains(wantVersions, version.Version) {
						var lockName string
						if len(wantVersions) > 1 {
							lockName = bzlRepoName(repo, chart, version.Version)
						} else {
							lockName = bzlRepoName(repo, chart, "")
						}

						lock.Repositories[lockName] = LockEntry{
							Chart:   chart,
							Version: version.Version,
							Digest:  version.Digest,
							Urls:    checkUrls(repo, version.Urls),
						}

						foundVersions[version.Version] = true
					}
				}

				if len(foundVersions) != len(wantVersions) {
					return nil, fmt.Errorf("did not find all versions of chart %s, found %v", chart, foundVersions)
				}
			}

		}

		if len(foundCharts) != len(wantCharts) {
			return nil, fmt.Errorf("did not find all charts for repository %s, found %v", repoIndex, foundCharts)
		}
	}

	return &lock, nil
}

func bzlRepoName(repo, chartName, version string) string {
	segments := make([]string, 0)

	repoUrl, err := url.Parse(repo)
	if err == nil {
		hostname := strings.ReplaceAll(repoUrl.Hostname(), "-", "_")
		parts := strings.Split(hostname, ".")
		slices.Reverse(parts)
		segments = append(segments, parts...)
	}

	segments = append(segments, strings.Split(chartName, "-")...)

	if version != "" {
		segments = append(segments, strings.Split(version, ".")...)
	}

	return strings.Join(segments, "_")
}

func checkUrls(repo string, urls []string) []string {
	checked := make([]string, len(urls))

	for idx, url := range urls {
		// Relative paths
		if strings.HasPrefix("/", url) {
			checked[idx] = fmt.Sprintf("%s%s", repo, url)
		} else if strings.HasPrefix(strings.ToLower(url), "http") {
			checked[idx] = url
		} else {
			checked[idx] = fmt.Sprintf("%s/%s", repo, url)
		}
	}
	return checked
}
