package chartmuseum

import (
	"time"
)

type (
	// Option allows specifying various settings
	Option func(*options)

	// options specify optional settings
	options struct {
		url                string
		clientID           string
		clientSecret       string
		contextPath        string
		timeout            time.Duration
		caFile             string
		certFile           string
		keyFile            string
		insecureSkipVerify bool
	}
)

// URL specifies the chart repo URL
func URL(url string) Option {
	return func(opts *options) {
		opts.url = url
	}
}

// ClientID is the Cloudflare Access client ID
func ClientID(clientID string) Option {
	return func(opts *options) {
		opts.clientID = clientID
	}
}

// ClientSecret is the Cloudflare Access client Secret
func ClientSecret(clientSecret string) Option {
	return func(opts *options) {
		opts.clientSecret = clientSecret
	}
}

// ContextPath is the URL prefix for ChartMuseum installation
func ContextPath(contextPath string) Option {
	return func(opts *options) {
		opts.contextPath = contextPath
	}
}

// Timeout specifies the duration (in seconds) before timing out request
func Timeout(timeout int64) Option {
	return func(opts *options) {
		opts.timeout = time.Duration(timeout) * time.Second
	}
}

//CAFile specifies the path of CA bundle
func CAFile(caFile string) Option {
	return func(opts *options) {
		opts.caFile = caFile
	}
}

//CertFile specifies the path of SSL certificate file
func CertFile(certFile string) Option {
	return func(opts *options) {
		opts.certFile = certFile
	}
}

//KeyFile specifies the path of SSL key file
func KeyFile(keyFile string) Option {
	return func(opts *options) {
		opts.keyFile = keyFile
	}
}

//InsecureSkipVerify to indicate if verify the certificate when connecting
func InsecureSkipVerify(insecureSkipVerify bool) Option {
	return func(opts *options) {
		opts.insecureSkipVerify = insecureSkipVerify
	}
}
