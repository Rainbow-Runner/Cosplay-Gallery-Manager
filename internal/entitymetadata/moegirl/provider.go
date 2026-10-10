package moegirl

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/antchfx/htmlquery"
	"github.com/stashapp/stash/internal/entitymetadata"
	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

var configStringPattern = func(key string) *regexp.Regexp {
	return regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `":"((?:\\.|[^"\\])*)"`)
}

var configNumberPattern = func(key string) *regexp.Regexp {
	return regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `":([0-9]+)`)
}

var bodyTemplatePattern = regexp.MustCompile(`(?is)<template\b[^>]*\bid=["']MOE_SKIN_TEMPLATE_BODYCONTENT["'][^>]*>`)

type fieldRule struct {
	category        string
	defaultSelected bool
}

var workFieldRules = map[string]fieldRule{
	"原名": {category: "original", defaultSelected: true}, "官方译名": {category: "official", defaultSelected: true},
	"外文名": {category: "foreign", defaultSelected: true}, "译名": {category: "translated", defaultSelected: true},
	"常用译名": {category: "common", defaultSelected: false}, "简称": {category: "short", defaultSelected: false},
	"其他名称": {category: "other", defaultSelected: false},
}

var characterFieldRules = map[string]fieldRule{
	"本名": {category: "original", defaultSelected: true}, "外文名": {category: "foreign", defaultSelected: true},
	"官方译名": {category: "official", defaultSelected: true}, "译名": {category: "translated", defaultSelected: true},
	"别名": {category: "alias", defaultSelected: false}, "别号": {category: "alias", defaultSelected: false},
	"昵称": {category: "nickname", defaultSelected: false},
}

func (p *Provider) Info() entitymetadata.ProviderInfo {
	return entitymetadata.ProviderInfo{Key: providerKey, Label: "萌娘百科"}
}

func (p *Provider) Search(ctx context.Context, request entitymetadata.SearchRequest) ([]entitymetadata.Candidate, error) {
	query := strings.TrimSpace(request.Query)
	if contextValue := strings.TrimSpace(request.Context); contextValue != "" {
		query += " " + contextValue
	}
	search := baseURL + "/Special:%E6%90%9C%E7%B4%A2?fulltext=1&namespace=0&search=" + url.QueryEscape(query)
	data, finalURL, err := p.get(ctx, search)
	if err != nil {
		return nil, err
	}
	return parseSearch(data, request.Query, finalURL), nil
}

func parseSearch(data []byte, query, finalURL string) []entitymetadata.Candidate {
	document, err := parseContentDocument(data)
	if err != nil {
		return []entitymetadata.Candidate{}
	}
	seen := make(map[string]struct{})
	result := make([]entitymetadata.Candidate, 0)
	add := func(title, contextValue string) {
		title = strings.TrimSpace(title)
		if title == "" || len([]rune(title)) > 300 {
			return
		}
		ref := encodeRef(title)
		if _, exists := seen[ref]; exists {
			return
		}
		seen[ref] = struct{}{}
		result = append(result, entitymetadata.Candidate{Ref: ref, DisplayName: title, Context: strings.TrimSpace(contextValue),
			SourceURL: articleURL(title), MatchQuality: matchQuality(query, title), TypeConfidence: "UNKNOWN"})
	}
	if title := pageConfigString(data, "wgPageName"); title != "" && pageConfigString(data, "wgAction") == "view" && !strings.HasPrefix(title, "Special:") {
		add(strings.ReplaceAll(title, "_", " "), "")
	}
	for _, item := range htmlquery.Find(document, "//li[contains(concat(' ',normalize-space(@class),' '),' mw-search-result ')]") {
		anchor := htmlquery.FindOne(item, ".//*[contains(concat(' ',normalize-space(@class),' '),' mw-search-result-heading ')]//a[@href]")
		if anchor == nil {
			anchor = htmlquery.FindOne(item, ".//a[@href]")
		}
		if anchor == nil {
			continue
		}
		title := strings.TrimSpace(attr(anchor, "title"))
		if title == "" {
			title = strings.TrimSpace(htmlquery.InnerText(anchor))
		}
		snippet := htmlquery.FindOne(item, ".//*[contains(concat(' ',normalize-space(@class),' '),' searchresult ')]")
		contextValue := ""
		if snippet != nil {
			contextValue = compactText(htmlquery.InnerText(snippet), 160)
		}
		add(title, contextValue)
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

func (p *Provider) FetchNames(ctx context.Context, kind entitymetadata.EntityKind, ref string) (entitymetadata.NameProfile, error) {
	if !kind.Valid() {
		return entitymetadata.NameProfile{}, errors.New("invalid Moegirlpedia entity kind")
	}
	title, err := decodeRef(ref)
	if err != nil {
		return entitymetadata.NameProfile{}, err
	}
	data, finalURL, err := p.get(ctx, articleURL(title))
	if err != nil {
		return entitymetadata.NameProfile{}, err
	}
	return parseProfile(data, kind, finalURL)
}

func parseProfile(data []byte, kind entitymetadata.EntityKind, sourceURL string) (entitymetadata.NameProfile, error) {
	document, err := parseContentDocument(data)
	if err != nil {
		return entitymetadata.NameProfile{}, err
	}
	displayName := strings.ReplaceAll(pageConfigString(data, "wgTitle"), "_", " ")
	if displayName == "" {
		displayName = strings.ReplaceAll(pageConfigString(data, "wgPageName"), "_", " ")
	}
	if heading := htmlquery.FindOne(document, "//*[@id='firstHeading']"); displayName == "" && heading != nil {
		displayName = strings.TrimSpace(htmlquery.InnerText(heading))
	}
	if displayName == "" || strings.Contains(compactText(string(data), 2000), "消歧义页") {
		return entitymetadata.NameProfile{}, errors.New("Moegirlpedia page is missing a usable subject title")
	}
	rules := workFieldRules
	if kind == entitymetadata.KindCharacter {
		rules = characterFieldRules
	}
	suggestions := extractSuggestions(document, displayName, rules)
	return entitymetadata.NameProfile{DisplayName: displayName, SourceURL: sourceURL,
		PageID: pageConfigNumber(data, "wgArticleId"), RevisionID: pageConfigNumber(data, "wgCurRevisionId"), Suggestions: suggestions}, nil
}

func parseContentDocument(data []byte) (*html.Node, error) {
	if body := bodyTemplate(data); len(body) != 0 {
		return htmlquery.Parse(bytes.NewReader(body))
	}
	return htmlquery.Parse(bytes.NewReader(data))
}

func bodyTemplate(data []byte) []byte {
	opening := bodyTemplatePattern.FindIndex(data)
	if len(opening) != 2 {
		return nil
	}
	closing := bytes.Index(bytes.ToLower(data[opening[1]:]), []byte("</template>"))
	if closing < 0 {
		return nil
	}
	return data[opening[1] : opening[1]+closing]
}

func extractSuggestions(document *html.Node, displayName string, rules map[string]fieldRule) []entitymetadata.AliasSuggestion {
	seen := map[string]struct{}{normalizedName(displayName): {}}
	result := make([]entitymetadata.AliasSuggestion, 0)
	var nodes []*html.Node
	var collect func(*html.Node)
	collect = func(node *html.Node) {
		if node.Type == html.ElementNode {
			nodes = append(nodes, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(document)
	for _, node := range nodes {
		label := strings.TrimSpace(htmlquery.InnerText(node))
		rule, ok := rules[label]
		if !ok || hasIgnoredAncestor(node) || !withinNameContainer(node) {
			continue
		}
		valueNode := adjacentValueNode(node)
		if valueNode == nil {
			continue
		}
		for _, value := range splitFieldValues(visibleText(valueNode)) {
			key := normalizedName(value)
			if key == "" || len([]rune(value)) > 300 {
				continue
			}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, entitymetadata.AliasSuggestion{Value: value, Category: rule.category,
				LanguageHint: languageHint(value), Evidence: label, DefaultSelected: rule.defaultSelected})
			if len(result) == 100 {
				return result
			}
		}
	}
	return result
}

func withinNameContainer(node *html.Node) bool {
	for current := node; current != nil; current = current.Parent {
		if current.Type != html.ElementNode {
			continue
		}
		if current.Data == "table" || strings.Contains(strings.ToLower(attr(current, "class")), "infobox") {
			return true
		}
	}
	return false
}

func adjacentValueNode(label *html.Node) *html.Node {
	for current, depth := label, 0; current != nil && depth < 4; current, depth = current.Parent, depth+1 {
		for sibling := current.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			if sibling.Type == html.TextNode && strings.TrimSpace(sibling.Data) == "" {
				continue
			}
			if sibling.Type == html.ElementNode {
				return sibling
			}
			break
		}
		if current.Type == html.ElementNode && (current.Data == "td" || current.Data == "th") {
			break
		}
	}
	return nil
}

func visibleText(root *html.Node) string {
	var builder strings.Builder
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && ignoredElement(node) {
			return
		}
		if node.Type == html.TextNode {
			builder.WriteString(node.Data)
		}
		if node.Type == html.ElementNode && node.Data == "br" {
			builder.WriteByte('\n')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
		if node.Type == html.ElementNode && (node.Data == "div" || node.Data == "p" || node.Data == "li") {
			builder.WriteByte('\n')
		}
	}
	visit(root)
	return builder.String()
}

func ignoredElement(node *html.Node) bool {
	if node.Type != html.ElementNode {
		return false
	}
	switch node.Data {
	case "del", "s", "strike", "script", "style", "sup", "rt", "template":
		return true
	}
	if attr(node, "hidden") != "" || strings.EqualFold(attr(node, "aria-hidden"), "true") {
		return true
	}
	classes := strings.ToLower(attr(node, "class"))
	return strings.Contains(classes, "heimu") || strings.Contains(classes, "hidden")
}

func hasIgnoredAncestor(node *html.Node) bool {
	for current := node; current != nil; current = current.Parent {
		if ignoredElement(current) {
			return true
		}
	}
	return false
}

func splitFieldValues(value string) []string {
	value = strings.NewReplacer("、", "\n", "，", "\n", ";", "\n", "；", "\n").Replace(value)
	parts := strings.Split(value, "\n")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(part, "日："), "英："), "中："))
		part = strings.TrimSpace(strings.Trim(part, "·•"))
		if part != "" {
			result = append(result, norm.NFC.String(part))
		}
	}
	return result
}

func pageConfigString(data []byte, key string) string {
	match := configStringPattern(key).FindSubmatch(data)
	if len(match) != 2 {
		return ""
	}
	value, err := strconv.Unquote(`"` + string(match[1]) + `"`)
	if err != nil {
		return ""
	}
	return value
}

func pageConfigNumber(data []byte, key string) string {
	match := configNumberPattern(key).FindSubmatch(data)
	if len(match) != 2 {
		return ""
	}
	return string(match[1])
}

func encodeRef(title string) string { return base64.RawURLEncoding.EncodeToString([]byte(title)) }

func decodeRef(ref string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(ref)
	title := strings.TrimSpace(string(data))
	if err != nil || title == "" || len([]rune(title)) > 300 || strings.IndexFunc(title, unicode.IsControl) >= 0 {
		return "", errors.New("invalid Moegirlpedia article reference")
	}
	return title, nil
}

func articleURL(title string) string {
	value, _ := url.Parse(baseURL)
	value.Path = "/" + strings.ReplaceAll(title, " ", "_")
	return value.String()
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
	return 20
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

func languageHint(value string) string {
	for _, character := range value {
		switch {
		case unicode.In(character, unicode.Hiragana, unicode.Katakana):
			return "ja"
		case unicode.In(character, unicode.Hangul):
			return "ko"
		case unicode.Is(unicode.Latin, character):
			return "latin"
		}
	}
	return ""
}

func compactText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	characters := []rune(value)
	if len(characters) > limit {
		value = string(characters[:limit])
	}
	return value
}

func attr(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}
	return ""
}

var _ entitymetadata.Provider = (*Provider)(nil)
