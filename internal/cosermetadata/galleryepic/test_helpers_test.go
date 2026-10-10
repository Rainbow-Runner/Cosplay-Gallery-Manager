package galleryepic

import (
	"net/netip"
	"net/url"
)

func mustURL(value string) *url.URL  { parsed, _ := url.Parse(value); return parsed }
func mustIP(value string) netip.Addr { return netip.MustParseAddr(value) }
