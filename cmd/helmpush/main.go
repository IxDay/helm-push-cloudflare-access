package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	cm "github.com/IxDay/helm-push-cloudflare-access/pkg/chartmuseum"
	"github.com/IxDay/helm-push-cloudflare-access/pkg/helm"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	v2downloader "k8s.io/helm/pkg/downloader"
	v2getter "k8s.io/helm/pkg/getter"
	v2environment "k8s.io/helm/pkg/helm/environment"
)

type (
	pushCmd struct {
		chartName          string
		appVersion         string
		chartVersion       string
		repoName           string
		clientID           string
		clientSecret       string
		contextPath        string
		forceUpload        bool
		useHTTP            bool
		checkHelmVersion   bool
		caFile             string
		certFile           string
		keyFile            string
		insecureSkipVerify bool
		keyring            string
		dependencyUpdate   bool
		out                io.Writer
	}

	config struct {
		CurrentContext string             `json:"current-context"`
		Contexts       map[string]context `json:"contexts"`
	}

	context struct {
		Name  string `json:"name"`
		Token string `json:"token"`
	}
)

var (
	v2settings  v2environment.EnvSettings
	settings    = cli.New()
	globalUsage = `Helm plugin to push chart package to ChartMuseum

Examples:

  $ helm push mychart-0.1.0.tgz chartmuseum       # push .tgz from "helm package"
  $ helm push . chartmuseum                       # package and push chart directory
  $ helm push . --version="7c4d121" chartmuseum   # override version in Chart.yaml
  $ helm push . https://my.chart.repo.com         # push directly to chart repo URL
`
)

func newPushCmd(args []string) *cobra.Command {
	p := &pushCmd{}
	cmd := &cobra.Command{
		Use:          "helm push",
		Short:        "Helm plugin to push chart package to ChartMuseum",
		Long:         globalUsage,
		SilenceUsage: false,
		RunE: func(cmd *cobra.Command, args []string) error {

			// If the --check-helm-version flag is provided, short circuit
			if p.checkHelmVersion {
				fmt.Println(helm.HelmMajorVersionCurrent())
				return nil
			}

			p.out = cmd.OutOrStdout()

			// If there are 4 args, this is likely being used as a downloader for cm:// protocol
			if len(args) == 4 && strings.HasPrefix(args[3], "cm://") {
				p.setFieldsFromEnv()
				return p.download(args[3])
			}

			if len(args) != 2 {
				return errors.New("This command needs 2 arguments: name of chart, name of chart repository (or repo URL)")
			}
			p.chartName = args[0]
			p.repoName = args[1]
			p.setFieldsFromEnv()
			return p.push()
		},
	}
	f := cmd.Flags()
	f.StringVarP(&p.chartVersion, "version", "v", "", "Override chart version pre-push")
	f.StringVarP(&p.appVersion, "app-version", "a", "", "Override app version pre-push")
	f.StringVarP(&p.clientID, "client-id", "", "", "Cloudflare access client ID [$HELM_REPO_CLIENT_ID]")
	f.StringVarP(&p.clientSecret, "client-secret", "", "", "Cloudflare access client secret [$HELM_REPO_CLIENT_SECRET]")
	f.StringVarP(&p.contextPath, "context-path", "", "", "ChartMuseum context path [$HELM_REPO_CONTEXT_PATH]")
	f.StringVarP(&p.caFile, "ca-file", "", "", "Verify certificates of HTTPS-enabled servers using this CA bundle [$HELM_REPO_CA_FILE]")
	f.StringVarP(&p.certFile, "cert-file", "", "", "Identify HTTPS client using this SSL certificate file [$HELM_REPO_CERT_FILE]")
	f.StringVarP(&p.keyFile, "key-file", "", "", "Identify HTTPS client using this SSL key file [$HELM_REPO_KEY_FILE]")
	f.StringVar(&p.keyring, "keyring", defaultKeyring(), "location of a public keyring")
	f.BoolVarP(&p.insecureSkipVerify, "insecure", "", false, "Connect to server with an insecure way by skipping certificate verification [$HELM_REPO_INSECURE]")
	f.BoolVarP(&p.forceUpload, "force", "f", false, "Force upload even if chart version exists")
	f.BoolVarP(&p.dependencyUpdate, "dependency-update", "d", false, `update dependencies from "requirements.yaml" to dir "charts/" before packaging`)
	f.BoolVarP(&p.checkHelmVersion, "check-helm-version", "", false, `outputs either "2" or "3" indicating the current Helm major version`)

	f.Parse(args)

	v2settings.AddFlags(f)
	v2settings.Init(f)

	return cmd
}

func (p *pushCmd) setFieldsFromEnv() {
	if v, ok := os.LookupEnv("HELM_REPO_CLIENT_ID"); ok && p.clientID == "" {
		p.clientID = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_CLIENT_SECRET"); ok && p.clientSecret == "" {
		p.clientSecret = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_CONTEXT_PATH"); ok && p.contextPath == "" {
		p.contextPath = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_USE_HTTP"); ok {
		p.useHTTP, _ = strconv.ParseBool(v)
	}
	if v, ok := os.LookupEnv("HELM_REPO_CA_FILE"); ok && p.caFile == "" {
		p.caFile = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_CERT_FILE"); ok && p.certFile == "" {
		p.certFile = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_KEY_FILE"); ok && p.keyFile == "" {
		p.keyFile = v
	}
	if v, ok := os.LookupEnv("HELM_REPO_INSECURE"); ok {
		p.insecureSkipVerify, _ = strconv.ParseBool(v)
	}
}

func (p *pushCmd) push() error {
	var repo *helm.Repo
	var err error

	// If the argument looks like a URL, just create a temp repo object
	// instead of looking for the entry in the local repository list
	if regexp.MustCompile(`^https?://`).MatchString(p.repoName) {
		repo, err = helm.TempRepoFromURL(p.repoName)
		p.repoName = repo.Config.URL
	} else {
		repo, err = helm.GetRepoByName(p.repoName)
	}

	if err != nil {
		return err
	}

	if p.dependencyUpdate {
		name := filepath.FromSlash(p.chartName)
		fi, err := os.Stat(name)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if validChart, err := chartutil.IsChartDir(name); !validChart {
				return err
			}
			chartPath, err := filepath.Abs(p.chartName)
			if err != nil {
				return err
			}
			if helm.HelmMajorVersionCurrent() == helm.HelmMajorVersion2 {
				v2downloadManager := &v2downloader.Manager{
					Out:       p.out,
					ChartPath: chartPath,
					HelmHome:  v2settings.Home,
					Keyring:   p.keyring,
					Getters:   v2getter.All(v2settings),
					Debug:     v2settings.Debug,
				}
				if err := v2downloadManager.Update(); err != nil {
					return err
				}
			} else {
				downloadManager := &downloader.Manager{
					Out:       p.out,
					ChartPath: chartPath,
					Keyring:   p.keyring,
					Getters:   getter.All(settings),
					Debug:     v2settings.Debug,
				}
				if err := downloadManager.Update(); err != nil {
					return err
				}
			}
		}
	}

	chart, err := helm.GetChartByName(p.chartName)
	if err != nil {
		return err
	}

	// version override
	if p.chartVersion != "" {
		chart.SetVersion(p.chartVersion)
	}

	// app version override
	if p.appVersion != "" {
		chart.SetAppVersion(p.appVersion)
	}

	// in case the repo is stored with cm:// protocol, remove it
	var url string
	if p.useHTTP {
		url = strings.Replace(repo.Config.URL, "cm://", "http://", 1)
	} else {
		url = strings.Replace(repo.Config.URL, "cm://", "https://", 1)
	}

	client, err := cm.NewClient(
		cm.URL(url),
		cm.ClientID(p.clientID),
		cm.ClientSecret(p.clientSecret),
		cm.ContextPath(p.contextPath),
		cm.CAFile(p.caFile),
		cm.CertFile(p.certFile),
		cm.KeyFile(p.keyFile),
		cm.InsecureSkipVerify(p.insecureSkipVerify),
	)

	if err != nil {
		return err
	}

	// update context path if not overrided
	if p.contextPath == "" {
		index, err := helm.GetIndexByRepo(repo, getIndexDownloader(client))
		if err != nil {
			return err
		}
		client.Option(cm.ContextPath(index.ServerInfo.ContextPath))
	}

	tmp, err := ioutil.TempDir("", "helm-push-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	chartPackagePath, err := helm.CreateChartPackage(chart, tmp)
	if err != nil {
		return err
	}

	fmt.Printf("Pushing %s to %s...\n", filepath.Base(chartPackagePath), p.repoName)
	resp, err := client.UploadChartPackage(chartPackagePath, p.forceUpload)
	if err != nil {
		return err
	}

	return handlePushResponse(resp)
}

func (p *pushCmd) download(fileURL string) error {
	parsedURL, err := url.Parse(fileURL)
	if err != nil {
		return err
	}

	parts := strings.Split(parsedURL.Path, "/")
	numParts := len(parts)
	if numParts <= 1 {
		return fmt.Errorf("invalid file url: %s", fileURL)
	}

	filePath := parts[numParts-1]

	numRemoveParts := 1
	if parts[numParts-2] == "charts" {
		numRemoveParts++
		filePath = "charts/" + filePath
	}

	parsedURL.Path = strings.Join(parts[:numParts-numRemoveParts], "/")

	if p.useHTTP {
		parsedURL.Scheme = "http"
	} else {
		parsedURL.Scheme = "https"
	}

	client, err := cm.NewClient(
		cm.URL(parsedURL.String()),
		cm.ClientID(p.clientID),
		cm.ClientSecret(p.clientSecret),
		cm.ContextPath(p.contextPath),
		cm.CAFile(p.caFile),
		cm.CertFile(p.certFile),
		cm.KeyFile(p.keyFile),
		cm.InsecureSkipVerify(p.insecureSkipVerify),
	)

	if err != nil {
		return err
	}

	resp, err := client.DownloadFile(filePath)
	if err != nil {
		return err
	}

	return handleDownloadResponse(resp)
}

func handlePushResponse(resp *http.Response) error {
	if resp.StatusCode != 201 {
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return getChartmuseumError(b, resp.StatusCode)
	}
	fmt.Println("Done.")
	return nil
}

func handleDownloadResponse(resp *http.Response) error {
	b, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return getChartmuseumError(b, resp.StatusCode)
	}
	fmt.Print(string(b))
	return nil
}

func getChartmuseumError(b []byte, code int) error {
	var er struct {
		Error string `json:"error"`
	}
	err := json.Unmarshal(b, &er)
	if err != nil || er.Error == "" {
		return fmt.Errorf("%d: could not properly parse response JSON: %s", code, string(b))
	}
	return fmt.Errorf("%d: %s", code, er.Error)
}

func getIndexDownloader(client *cm.Client) helm.IndexDownloader {
	return func() ([]byte, error) {
		resp, err := client.DownloadFile("index.yaml")
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			return nil, getChartmuseumError(b, resp.StatusCode)
		}
		return b, nil
	}
}

func main() {
	cmd := newPushCmd(os.Args[1:])
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// defaultKeyring returns the expanded path to the default keyring.
func defaultKeyring() string {
	return os.ExpandEnv("$HOME/.gnupg/pubring.gpg")
}
