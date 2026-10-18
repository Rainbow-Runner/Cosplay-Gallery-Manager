// Package galleryepic implements the optional GalleryEpic Coser metadata
// provider. It is the only package allowed to know GalleryEpic hosts, routes,
// HTML structure, and social-link presentation details.
package galleryepic

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/stashapp/stash/internal/cosermetadata"
)

const (
	providerKey      = "galleryepic"
	baseURL          = "https://galleryepic.xyz"
	maxHTMLBytes     = int64(4 * 1024 * 1024)
	requestTimeout   = 15 * time.Second
	operationTimeout = 30 * time.Second
)

type Provider struct {
	client *http.Client
}

func New() *Provider {
	return &Provider{client: safeClient()}
}

func NewWithClient(client *http.Client) *Provider {
	return &Provider{client: client}
}

func (p *Provider) Info() cosermetadata.ProviderInfo {
	return cosermetadata.ProviderInfo{Key: providerKey, Label: "GalleryEpic"}
}

func safeClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, address := range addresses {
				if !publicIP(address) {
					return nil, errors.New("GalleryEpic host resolved to a non-public address")
				}
			}
			if len(addresses) == 0 {
				return nil, errors.New("GalleryEpic host did not resolve")
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
		},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		DisableCompression:    false,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   2,
	}
	return &http.Client{Transport: transport, Timeout: operationTimeout, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many GalleryEpic redirects")
		}
		return validateRemoteURL(request.URL, remoteHosts)
	}}
}

var pageHosts = map[string]bool{
	"galleryepic.com": true, "www.galleryepic.com": true,
	"galleryepic.xyz": true, "www.galleryepic.xyz": true,
}

var imageHosts = map[string]bool{"static.galleryepic.xyz": true}

var remoteHosts = map[string]bool{
	"galleryepic.com": true, "www.galleryepic.com": true,
	"galleryepic.xyz": true, "www.galleryepic.xyz": true,
	"static.galleryepic.xyz": true,
}

func publicIP(value netip.Addr) bool {
	value = value.Unmap()
	return value.IsValid() && !value.IsLoopback() && !value.IsPrivate() && !value.IsLinkLocalUnicast() &&
		!value.IsLinkLocalMulticast() && !value.IsMulticast() && !value.IsUnspecified()
}

func validateRemoteURL(value *url.URL, hosts map[string]bool) error {
	if value == nil || strings.ToLower(value.Scheme) != "https" || value.User != nil || value.Port() != "" || !hosts[strings.ToLower(value.Hostname())] {
		return errors.New("remote metadata URL is not allowed")
	}
	return nil
}

func (p *Provider) get(ctx context.Context, rawURL string, hosts map[string]bool, maxBytes int64, contentTypePrefix string) ([]byte, string, error) {
	if p == nil || p.client == nil {
		return nil, "", errors.New("GalleryEpic client is unavailable")
	}
	value, err := url.Parse(rawURL)
	if err != nil || validateRemoteURL(value, hosts) != nil {
		return nil, "", errors.New("GalleryEpic request URL is invalid")
	}
	requestContext, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, value.String(), nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("Accept", contentTypePrefix+"/*")
	request.Header.Set("User-Agent", "Cosplay-Gallery-Manager/1.5 metadata-import")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.Request == nil || validateRemoteURL(response.Request.URL, hosts) != nil {
		return nil, "", errors.New("GalleryEpic redirected outside the allowed resource hosts")
	}
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("GalleryEpic returned HTTP %d", response.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if !strings.HasPrefix(contentType, contentTypePrefix+"/") || contentTypePrefix == "text" && contentType != "text/html" {
		return nil, "", errors.New("GalleryEpic returned an unexpected content type")
	}
	if response.ContentLength > maxBytes {
		return nil, "", errors.New("GalleryEpic response exceeds the allowed size")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || int64(len(data)) > maxBytes {
		return nil, "", errors.New("GalleryEpic response is empty or too large")
	}
	return data, contentType, nil
}
