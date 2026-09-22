package galleryepic

import (
	"os"
	"testing"
)

func TestParseCurrentGalleryEpicProfileShape(t *testing.T) {
	html := `<html><body><img src="https://static.galleryepic.xyz/image/banner" alt="298"><img src="https://static.galleryepic.xyz/image/avatar" alt="298">
	<div><h4 class="mb-2 text-xl font-semibold text-white">Yaokoututu</h4><div class="flex items-center space-x-1">
	<a href="https://twitter.com/yaokoututu" title="X">X</a><template id="P:11"></template><template id="P:12"></template></div></div>
	<div hidden id="S:11"><a href="https://www.patreon.com/yaokoututu" title="Patreon">P</a></div><script>$RS("S:11","P:11")</script>
	<div hidden id="S:12"><a href="https://linktr.ee/yaokoututu" title="Linktree">L</a></div><script>$RS("S:12","P:12")</script>
	<footer><a href="https://twitter.com/site-account" title="X">unrelated site account</a></footer></body></html>`
	profile, err := parseProfile([]byte(html), "298", "https://galleryepic.xyz/en/coser/298/1")
	if err != nil {
		t.Fatal(err)
	}
	if profile.DisplayName != "Yaokoututu" || profile.Avatar.Ref != "https://static.galleryepic.xyz/image/avatar" ||
		profile.Banner.Ref != "https://static.galleryepic.xyz/image/banner" || len(profile.Accounts) != 3 {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestParseSearchUsesOpaqueNumericReference(t *testing.T) {
	html := `<a href="/zh/coser/298/1"><img alt="298">咬一口兔娘</a><a href="/zh/coser/298/2">duplicate</a>`
	candidates := parseSearch([]byte(html), "咬一口兔娘", mustURL("https://galleryepic.xyz/zh/cosers/1"))
	if len(candidates) != 1 || candidates[0].Ref != "298" || candidates[0].MatchQuality != 100 {
		t.Fatalf("candidates = %#v", candidates)
	}
}

func TestOnlyAllowlistedSocialHostsAreImported(t *testing.T) {
	if account, ok := socialAccount("Weibo", "https://weibo.com/u/7815982641"); !ok || account.PlatformKey != "weibo" || account.Handle != "7815982641" {
		t.Fatalf("weibo = %#v, %v", account, ok)
	}
	if _, ok := socialAccount("Website", "https://evil.test/profile"); ok {
		t.Fatal("unknown social host accepted")
	}
}

func TestPublicIPRejectsLocalNetworks(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "::1", "fe80::1"} {
		if publicIP(mustIP(value)) {
			t.Fatalf("private IP %s accepted", value)
		}
	}
	if !publicIP(mustIP("1.1.1.1")) {
		t.Fatal("public IP rejected")
	}
}

func TestFixtureCanBeReplacedWithoutCoreDependency(t *testing.T) {
	data, err := os.ReadFile("testdata/profile.html")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := parseProfile(data, "1", "https://galleryepic.xyz/en/coser/1/1")
	if err != nil || profile.DisplayName != "Fixture Coser" || profile.Avatar == nil || profile.Banner == nil {
		t.Fatalf("fixture profile = %#v, %v", profile, err)
	}
}
