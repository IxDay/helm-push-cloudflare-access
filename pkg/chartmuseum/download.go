package chartmuseum

import (
	"net/http"
	"net/url"
	"path"
	"strings"
)

// DownloadFile downloads a file from ChartMuseum
func (client *Client) DownloadFile(filePath string) (*http.Response, error) {
	u, err := url.Parse(client.opts.url)
	if err != nil {
		return nil, err
	}

	u.Path = path.Join(client.opts.contextPath, strings.TrimPrefix(u.Path, client.opts.contextPath), filePath)
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set(cfHeaderId, client.opts.clientID)
	req.Header.Set(cfHeaderSecret, client.opts.clientSecret)

	return client.Do(req)
}
