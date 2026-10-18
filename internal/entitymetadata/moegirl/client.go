// Package moegirl implements the optional Moegirlpedia Work and Character
// name provider. It is the only package allowed to know Moegirlpedia hosts,
// routes, or HTML structure.
package moegirl

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
)

const (
	providerKey      = "moegirl"
	baseURL          = "https://zh.moegirl.org.cn"
	maxHTMLBytes     = int64(8 * 1024 * 1024)
	requestTimeout   = 15 * time.Second
	operationTimeout = 30 * time.Second
)

var pageHosts = map[string]bool{"zh.moegirl.org.cn": true}

type Provider struct{ client *http.Client }

func New() *Provider { return &Provider{client: safeClient()} }

func NewWithClient(client *http.Client) *Provider { return &Provider{client: client} }

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
					return nil, errors.New("Moegirlpedia host resolved to a non-public address")
				}
			}
			if len(addresses) == 0 {
				return nil, errors.New("Moegirlpedia host did not resolve")
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
		},
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout: 30 * time.Second, MaxIdleConns: 4, MaxIdleConnsPerHost: 2,
	}
	return &http.Client{Transport: transport, Timeout: operationTimeout, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many Moegirlpedia redirects")
		}
		return validateRemoteURL(request.URL)
	}}
}

func publicIP(value netip.Addr) bool {
	value = value.Unmap()
	return value.IsValid() && !value.IsLoopback() && !value.IsPrivate() && !value.IsLinkLocalUnicast() &&
		!value.IsLinkLocalMulticast() && !value.IsMulticast() && !value.IsUnspecified()
}

func validateRemoteURL(value *url.URL) error {
	if value == nil || strings.ToLower(value.Scheme) != "https" || value.User != nil || value.Port() != "" || !pageHosts[strings.ToLower(value.Hostname())] {
		return errors.New("remote metadata URL is not allowed")
	}
	return nil
}

func (p *Provider) get(ctx context.Context, rawURL string) ([]byte, string, error) {
	if p == nil || p.client == nil {
		return nil, "", errors.New("Moegirlpedia client is unavailable")
	}
	value, err := url.Parse(rawURL)
	if err != nil || validateRemoteURL(value) != nil {
		return nil, "", errors.New("Moegirlpedia request URL is invalid")
	}
	requestContext, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, value.String(), nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("Accept", "text/html")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	request.Header.Set("User-Agent", "Cosplay-Gallery-Manager/1.5 optional-name-metadata-import")
	response, err := p.client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.Request == nil || validateRemoteURL(response.Request.URL) != nil {
		return nil, "", errors.New("Moegirlpedia redirected outside the allowed host")
	}
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("Moegirlpedia returned HTTP %d", response.StatusCode)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if contentType != "text/html" {
		return nil, "", errors.New("Moegirlpedia returned an unexpected content type")
	}
	if response.ContentLength > maxHTMLBytes {
		return nil, "", errors.New("Moegirlpedia response exceeds the allowed size")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxHTMLBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || int64(len(data)) > maxHTMLBytes {
		return nil, "", errors.New("Moegirlpedia response is empty or too large")
	}
	return data, response.Request.URL.String(), nil
}
