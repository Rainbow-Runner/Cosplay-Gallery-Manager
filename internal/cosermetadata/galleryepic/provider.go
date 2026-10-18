package galleryepic

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/antchfx/htmlquery"
	"github.com/stashapp/stash/internal/cosermetadata"
	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

var coserPathPattern = regexp.MustCompile(`^/(?:zh|en)/coser/([1-9][0-9]*)/[1-9][0-9]*$`)
var coserRefPattern = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)

func (p *Provider) Search(ctx context.Context, query string) ([]cosermetadata.Candidate, error) {
	search, _ := url.Parse(baseURL + "/zh/cosers/1")
	parameters := search.Query()
	parameters.Set("coserName", strings.TrimSpace(query))
	search.RawQuery = parameters.Encode()
	data, _, err := p.get(ctx, search.String(), pageHosts, maxHTMLBytes, "text")
	if err != nil {
		return nil, err
	}
	return parseSearch(data, query, search), nil
}

func parseSearch(data []byte, query string, base *url.URL) []cosermetadata.Candidate {
	document, err := htmlquery.Parse(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var result []cosermetadata.Candidate
	for _, anchor := range htmlquery.Find(document, "//a[@href]") {
		href := attr(anchor, "href")
		resolved, err := base.Parse(href)
		if err != nil || !pageHosts[strings.ToLower(resolved.Hostname())] {
			continue
		}
		match := coserPathPattern.FindStringSubmatch(resolved.Path)
		if len(match) != 2 || seen[match[1]] {
			continue
		}
		name := strings.TrimSpace(htmlquery.InnerText(anchor))
		if name == "" {
			continue
		}
		seen[match[1]] = true
		canonical := baseURL + "/en/coser/" + match[1] + "/1"
		result = append(result, cosermetadata.Candidate{Ref: match[1], DisplayName: name, SourceURL: canonical, MatchQuality: matchQuality(query, name)})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].MatchQuality != result[j].MatchQuality {
			return result[i].MatchQuality > result[j].MatchQuality
		}
		return result[i].DisplayName < result[j].DisplayName
	})
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

func (p *Provider) FetchProfile(ctx context.Context, ref string) (cosermetadata.Profile, error) {
	if !coserRefPattern.MatchString(ref) {
		return cosermetadata.Profile{}, errors.New("invalid GalleryEpic Coser reference")
	}
	if _, err := strconv.ParseUint(ref, 10, 64); err != nil {
		return cosermetadata.Profile{}, errors.New("invalid GalleryEpic Coser reference")
	}
	sourceURL := baseURL + "/en/coser/" + ref + "/1"
	data, _, err := p.get(ctx, sourceURL, pageHosts, maxHTMLBytes, "text")
	if err != nil {
		return cosermetadata.Profile{}, err
	}
	profile, err := parseProfile(data, ref, sourceURL)
	if err != nil {
		return cosermetadata.Profile{}, err
	}
	return profile, nil
}

func parseProfile(data []byte, ref, sourceURL string) (cosermetadata.Profile, error) {
	document, err := htmlquery.Parse(bytes.NewReader(data))
	if err != nil {
		return cosermetadata.Profile{}, err
	}
	profile := cosermetadata.Profile{SourceURL: sourceURL}
	var profileHeading *html.Node
	for _, heading := range htmlquery.Find(document, "//h4") {
		classes := attr(heading, "class")
		if strings.Contains(classes, "text-xl") && strings.Contains(classes, "font-semibold") {
			profile.DisplayName = strings.TrimSpace(htmlquery.InnerText(heading))
			if profile.DisplayName != "" {
				profileHeading = heading
				break
			}
		}
	}
	var images []string
	for _, image := range htmlquery.Find(document, "//img[@src]") {
		if attr(image, "alt") != ref {
			continue
		}
		value, err := url.Parse(attr(image, "src"))
		if err == nil && validateRemoteURL(value, imageHosts) == nil {
			images = append(images, value.String())
		}
	}
	if len(images) >= 1 {
		profile.Banner = &cosermetadata.RemoteAsset{Ref: images[0]}
	}
	if len(images) >= 2 {
		profile.Avatar = &cosermetadata.RemoteAsset{Ref: images[1]}
	}
	accounts := make(map[string]cosermetadata.SocialAccount)
	for _, anchor := range profileSocialAnchors(document, profileHeading) {
		account, ok := socialAccount(attr(anchor, "title"), attr(anchor, "href"))
		if ok {
			accounts[account.URL] = account
		}
	}
	for _, account := range accounts {
		profile.Accounts = append(profile.Accounts, account)
	}
	sort.Slice(profile.Accounts, func(i, j int) bool {
		if profile.Accounts[i].PlatformKey == profile.Accounts[j].PlatformKey {
			return profile.Accounts[i].URL < profile.Accounts[j].URL
		}
		return profile.Accounts[i].PlatformKey < profile.Accounts[j].PlatformKey
	})
	if profile.DisplayName == "" || profile.Avatar == nil {
		return cosermetadata.Profile{}, errors.New("GalleryEpic Coser profile did not contain the required name and avatar")
	}
	return profile, nil
}

func profileSocialAnchors(document, heading *html.Node) []*html.Node {
	if document == nil || heading == nil || heading.Parent == nil {
		return nil
	}
	var socialRoot *html.Node
	for child := heading.NextSibling; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "div" && hasClasses(child, "flex", "items-center", "space-x-1") {
			socialRoot = child
			break
		}
	}
	if socialRoot == nil {
		return nil
	}
	result := htmlquery.Find(socialRoot, ".//a[@href]")
	targets := make(map[string]bool)
	for _, template := range htmlquery.Find(socialRoot, ".//template[@id]") {
		targets[attr(template, "id")] = true
	}
	for _, hidden := range htmlquery.Find(document, "//*[@hidden and @id]") {
		sourceID := attr(hidden, "id")
		for sibling := hidden.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			if sibling.Type == html.TextNode {
				continue
			}
			if sibling.Type != html.ElementNode || sibling.Data != "script" {
				break
			}
			match := streamedTargetPattern.FindStringSubmatch(nodeText(sibling))
			if len(match) == 3 && match[1] == sourceID && targets[match[2]] {
				result = append(result, htmlquery.Find(hidden, ".//a[@href]")...)
			}
			break
		}
	}
	return result
}

var streamedTargetPattern = regexp.MustCompile(`\$RS\("(S:[A-Za-z0-9]+)","(P:[A-Za-z0-9]+)"\)`)

func nodeText(node *html.Node) string {
	var builder strings.Builder
	var visit func(*html.Node)
	visit = func(value *html.Node) {
		if value.Type == html.TextNode {
			builder.WriteString(value.Data)
		}
		for child := value.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return builder.String()
}

func hasClasses(node *html.Node, required ...string) bool {
	values := make(map[string]bool)
	for _, value := range strings.Fields(attr(node, "class")) {
		values[value] = true
	}
	for _, value := range required {
		if !values[value] {
			return false
		}
	}
	return true
}

func (p *Provider) OpenAsset(ctx context.Context, ref string) (cosermetadata.Asset, error) {
	data, contentType, err := p.get(ctx, ref, imageHosts, cosermetadata.MaxAssetBytes, "image")
	if err != nil {
		return cosermetadata.Asset{}, err
	}
	return cosermetadata.Asset{Reader: cosermetadata.AssetReader(data), ContentType: contentType, ByteSize: int64(len(data))}, nil
}

func socialAccount(title, rawURL string) (cosermetadata.SocialAccount, bool) {
	value, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || value.Scheme != "https" || value.User != nil || value.Hostname() == "" {
		return cosermetadata.SocialAccount{}, false
	}
	host := strings.TrimPrefix(strings.ToLower(value.Hostname()), "www.")
	platform, label := "", strings.TrimSpace(title)
	switch host {
	case "twitter.com", "x.com":
		platform, label = "twitter", "Twitter / X"
	case "instagram.com":
		platform, label = "instagram", "Instagram"
	case "weibo.com":
		platform, label = "weibo", "Weibo"
	case "patreon.com":
		platform, label = "patreon", "Patreon"
	case "facebook.com":
		platform, label = "facebook", "Facebook"
	case "youtube.com", "youtu.be":
		platform, label = "youtube", "YouTube"
	case "pixiv.net":
		platform, label = "pixiv", "Pixiv"
	case "bilibili.com":
		platform, label = "bilibili", "Bilibili"
	case "tiktok.com":
		platform, label = "tiktok", "TikTok"
	case "linktr.ee":
		platform, label = "website", "Linktree"
	default:
		return cosermetadata.SocialAccount{}, false
	}
	value.Fragment = ""
	handle := strings.Trim(path.Clean(value.Path), "/")
	if handle == "." {
		handle = ""
	}
	if strings.HasPrefix(handle, "u/") {
		handle = strings.TrimPrefix(handle, "u/")
	}
	return cosermetadata.SocialAccount{PlatformKey: platform, Label: label, Handle: handle, URL: value.String()}, true
}

func matchQuality(query, name string) int {
	left, right := normalizedName(query), normalizedName(name)
	if left == "" || right == "" {
		return 0
	}
	if left == right {
		return 100
	}
	if strings.HasPrefix(right, left) || strings.HasPrefix(left, right) {
		return 80
	}
	if strings.Contains(right, left) || strings.Contains(left, right) {
		return 60
	}
	leftSet, rightSet := make(map[rune]bool), make(map[rune]bool)
	for _, value := range left {
		leftSet[value] = true
	}
	for _, value := range right {
		rightSet[value] = true
	}
	intersection := 0
	for value := range leftSet {
		if rightSet[value] {
			intersection++
		}
	}
	union := len(leftSet) + len(rightSet) - intersection
	if union == 0 {
		return 0
	}
	return 40 * intersection / union
}

func normalizedName(value string) string {
	value = strings.ToLower(norm.NFC.String(strings.TrimSpace(value)))
	return strings.Map(func(value rune) rune {
		if unicode.IsLetter(value) || unicode.IsNumber(value) {
			return value
		}
		return -1
	}, value)
}

func attr(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

var _ cosermetadata.Provider = (*Provider)(nil)
