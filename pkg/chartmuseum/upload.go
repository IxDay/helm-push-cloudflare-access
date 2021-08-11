package chartmuseum

import (
	"bytes"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

const (
	cfHeaderId     = "CF-Access-Client-Id"
	cfHeaderSecret = "CF-Access-Client-Secret"
)

// UploadChartPackage uploads a chart package to ChartMuseum (POST /api/charts)
func (client *Client) UploadChartPackage(chartPackagePath string, force bool) (*http.Response, error) {
	u, err := url.Parse(client.opts.url)
	if err != nil {
		return nil, err
	}

	u.Path = path.Join(client.opts.contextPath, "api", strings.TrimPrefix(u.Path, client.opts.contextPath), "charts")
	req, err := http.NewRequest("POST", u.String(), nil)
	if err != nil {
		return nil, err
	}

	// Add ?force to request querystring to force an upload if chart version already exists
	if force {
		req.URL.RawQuery = "force"
	}

	err = setUploadChartPackageRequestBody(req, chartPackagePath)
	if err != nil {
		return nil, err
	}

	req.Header.Set(cfHeaderId, client.opts.clientID)
	req.Header.Set(cfHeaderSecret, client.opts.clientSecret)
	return client.Do(req)
}

func setUploadChartPackageRequestBody(req *http.Request, chartPackagePath string) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	defer w.Close()
	fw, err := w.CreateFormFile("chart", chartPackagePath)
	if err != nil {
		return err
	}
	w.FormDataContentType()
	fd, err := os.Open(chartPackagePath)
	if err != nil {
		return err
	}
	defer fd.Close()
	_, err = io.Copy(fw, fd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Body = ioutil.NopCloser(&body)
	return nil
}
