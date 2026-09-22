package moegirl

import (
	"net/netip"
	"os"
	"reflect"
	"testing"

	"github.com/stashapp/stash/internal/entitymetadata"
)

func TestOptionalLiveFixtureCompatibility(t *testing.T) {
	path := os.Getenv("CGM_MOEGIRL_LIVE_FIXTURE")
	if path == "" {
		t.Skip("set CGM_MOEGIRL_LIVE_FIXTURE to validate a separately downloaded public page")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := parseProfile(data, entitymetadata.KindWork, "https://zh.moegirl.org.cn/example")
	if err != nil {
		t.Fatal(err)
	}
	if profile.DisplayName == "" || profile.PageID == "" || profile.RevisionID == "" || len(profile.Suggestions) == 0 {
		t.Fatalf("live-compatible profile did not expose identity and names: %#v", profile)
	}
}

func TestOptionalLiveSearchFixtureCompatibility(t *testing.T) {
	path := os.Getenv("CGM_MOEGIRL_LIVE_SEARCH_FIXTURE")
	if path == "" {
		t.Skip("set CGM_MOEGIRL_LIVE_SEARCH_FIXTURE to validate a separately downloaded public search page")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	candidates := parseSearch(data, "大凤", "https://zh.moegirl.org.cn/Special:搜索")
	if len(candidates) == 0 || candidates[0].Ref == "" || candidates[0].DisplayName == "" {
		t.Fatalf("live-compatible search did not expose candidates: %#v", candidates)
	}
}

func TestOptionalLiveCharacterFixtureCompatibility(t *testing.T) {
	path := os.Getenv("CGM_MOEGIRL_LIVE_CHARACTER_FIXTURE")
	if path == "" {
		t.Skip("set CGM_MOEGIRL_LIVE_CHARACTER_FIXTURE to validate a separately downloaded public Character page")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := parseProfile(data, entitymetadata.KindCharacter, "https://zh.moegirl.org.cn/example")
	if err != nil {
		t.Fatal(err)
	}
	if profile.DisplayName == "" || len(profile.Suggestions) == 0 {
		t.Fatalf("live-compatible Character profile did not expose names: %#v", profile)
	}
}

func TestParseWorkNamesFromSemanticFlexRows(t *testing.T) {
	html := `<html><head><script>RLCONF={"wgTitle":"鸣潮","wgPageName":"鸣潮","wgArticleId":123,"wgCurRevisionId":456};</script></head><body>
	<h1 id="firstHeading">鸣潮</h1><div class="infobox">
	<div class="row"><div><span>原名</span></div><div>Wuthering Waves<br><ruby>鳴潮<rt>めいちょう</rt></ruby></div></div>
	<div class="row"><div><span>常用译名</span></div><div>鸣潮手游、<del>不要导入</del><span class="heimu">隐藏名</span>库洛鸣潮</div></div>
	</div></body></html>`
	profile, err := parseProfile([]byte(html), entitymetadata.KindWork, "https://zh.moegirl.org.cn/example")
	if err != nil {
		t.Fatal(err)
	}
	if profile.DisplayName != "鸣潮" || profile.PageID != "123" || profile.RevisionID != "456" {
		t.Fatalf("profile identity = %#v", profile)
	}
	values := make([]string, 0, len(profile.Suggestions))
	for _, suggestion := range profile.Suggestions {
		values = append(values, suggestion.Value)
		if suggestion.Value == "常用译名" || suggestion.Value == "不要导入" || suggestion.Value == "隐藏名" || suggestion.Value == "めいちょう" {
			t.Fatalf("excluded presentation text was imported: %#v", suggestion)
		}
	}
	if !reflect.DeepEqual(values, []string{"Wuthering Waves", "鳴潮", "鸣潮手游", "库洛鸣潮"}) {
		t.Fatalf("suggestions = %#v", profile.Suggestions)
	}
	if !profile.Suggestions[0].DefaultSelected || profile.Suggestions[2].DefaultSelected {
		t.Fatalf("selection defaults = %#v", profile.Suggestions)
	}
}

func TestParseCharacterNamesFromTableRows(t *testing.T) {
	html := `<script>RLCONF={"wgTitle":"大凤","wgPageName":"碧蓝航线:大凤","wgArticleId":77,"wgCurRevisionId":88};</script>
	<table><tr><td>本名</td><td>大鳳</td></tr><tr><th>别号</th><td>鹩；Taihou</td></tr></table>`
	profile, err := parseProfile([]byte(html), entitymetadata.KindCharacter, "https://zh.moegirl.org.cn/example")
	if err != nil {
		t.Fatal(err)
	}
	values := []string{}
	for _, suggestion := range profile.Suggestions {
		values = append(values, suggestion.Value)
	}
	if !reflect.DeepEqual(values, []string{"大鳳", "鹩", "Taihou"}) {
		t.Fatalf("suggestions = %#v", profile.Suggestions)
	}
}

func TestSearchSupportsResultsAndRedirectedExactArticle(t *testing.T) {
	results := `<li class="mw-search-result"><div class="mw-search-result-heading"><a title="琴·古恩希尔德" href="/琴">琴·古恩希尔德</a></div><div class="searchresult">角色介绍</div></li>`
	candidates := parseSearch([]byte(results), "琴", "https://zh.moegirl.org.cn/Special:搜索")
	if len(candidates) != 1 || candidates[0].DisplayName != "琴·古恩希尔德" || candidates[0].Context != "角色介绍" {
		t.Fatalf("search candidates = %#v", candidates)
	}
	exact := `<script>RLCONF={"wgAction":"view","wgPageName":"鸣潮"};</script>`
	candidates = parseSearch([]byte(exact), "鸣潮", "https://zh.moegirl.org.cn/鸣潮")
	if len(candidates) != 1 || candidates[0].MatchQuality != 100 {
		t.Fatalf("exact candidate = %#v", candidates)
	}
	if decoded, err := decodeRef(candidates[0].Ref); err != nil || decoded != "鸣潮" {
		t.Fatalf("opaque ref decoded as %q, %v", decoded, err)
	}
}

func TestMoegirlPublicIPRejectsLocalNetworks(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "::1", "fe80::1"} {
		if publicIP(netip.MustParseAddr(value)) {
			t.Fatalf("private IP %s accepted", value)
		}
	}
	if !publicIP(netip.MustParseAddr("1.1.1.1")) {
		t.Fatal("public IP rejected")
	}
}
