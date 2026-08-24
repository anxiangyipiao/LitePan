package rss

import (
	"strings"
	"testing"
)

func TestParseFeedRSS2MagnetEnclosure(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
<title>测试源</title>
<item>
  <guid>abc-1</guid>
  <title>孤独摇滚 第2话 1080p</title>
  <link>https://example.com/view/1</link>
  <pubDate>Mon, 02 Jan 2026 15:04:05 +0800</pubDate>
  <enclosure url="magnet:?xt=urn:btih:abcdef0123456789ABCDEF0123456789ABCDEF01&amp;dn=test" type="application/x-bittorrent" />
</item>
<item>
  <title>无guid用link</title>
  <link>https://example.com/view/2</link>
  <pubDate>Tue, 03 Jan 2026 15:04:05 +0800</pubDate>
  <enclosure url="https://example.com/foo.torrent" type="application/x-bittorrent" />
</item>
<item>
  <guid>desc-magnet</guid>
  <title>描述内嵌磁力</title>
  <description><![CDATA[<a href="magnet:?xt=urn:btih:1111222233334444555566667777888899990000&amp;tr=udp://x">下载</a>]]></description>
</item>
</channel></rss>`

	title, items, err := ParseFeed([]byte(feed))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if title != "测试源" {
		t.Fatalf("unexpected feed title: %q", title)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	it := items[0]
	if it.GUID != "abc-1" {
		t.Errorf("guid: %q", it.GUID)
	}
	if !isMagnet(it.TorrentURL) {
		t.Errorf("expected magnet torrent url, got %q", it.TorrentURL)
	}
	if it.InfoHash != "ABCDEF0123456789ABCDEF0123456789ABCDEF01" {
		t.Errorf("infohash: %q", it.InfoHash)
	}
	if it.PubDate.IsZero() {
		t.Error("pubdate should parse")
	}

	it2 := items[1]
	if it2.GUID != "https://example.com/view/2" {
		t.Errorf("guid fallback to link: %q", it2.GUID)
	}
	if it2.TorrentURL != "https://example.com/foo.torrent" {
		t.Errorf("http torrent enclosure: %q", it2.TorrentURL)
	}
	if it2.InfoHash != "" {
		t.Errorf("http torrent should have no infohash, got %q", it2.InfoHash)
	}

	it3 := items[2]
	if !strings.HasPrefix(it3.TorrentURL, "magnet:?xt=urn:btih:1111222233334444555566667777888899990000") {
		t.Errorf("magnet from description: %q", it3.TorrentURL)
	}
	if it3.InfoHash != "1111222233334444555566667777888899990000" {
		t.Errorf("desc infohash: %q", it3.InfoHash)
	}
}

func TestParseFeedAtom(t *testing.T) {
	feed := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<title>Atom 源</title>
<entry>
  <id>atom-1</id>
  <title>动画 第3话</title>
  <link rel="alternate" href="https://example.com/a" />
  <link rel="enclosure" href="magnet:?xt=urn:btih:9999999999999999999999999999999999999999" />
  <published>2026-01-02T15:04:05Z</published>
</entry>
<entry>
  <title>无 id 条目</title>
  <link href="https://example.com/b" />
  <summary>跳过</summary>
</entry>
</feed>`

	title, items, err := ParseFeed([]byte(feed))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if title != "Atom 源" {
		t.Fatalf("feed title: %q", title)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(items))
	}
	it := items[0]
	if it.GUID != "atom-1" {
		t.Errorf("guid: %q", it.GUID)
	}
	// enclosure link 优先于 alternate
	if !isMagnet(it.TorrentURL) {
		t.Errorf("atom magnet: %q", it.TorrentURL)
	}
	if it.InfoHash != "9999999999999999999999999999999999999999" {
		t.Errorf("atom infohash: %q", it.InfoHash)
	}
	if it.PubDate.IsZero() {
		t.Error("atom published should parse")
	}
	if it.Link != "https://example.com/a" {
		t.Errorf("atom link: %q", it.Link)
	}
	if items[1].GUID != "https://example.com/b" {
		t.Errorf("atom id fallback: %q", items[1].GUID)
	}
}

func TestParseFeedErrors(t *testing.T) {
	if _, _, err := ParseFeed([]byte("<html><body>not a feed</body></html>")); err == nil {
		t.Fatal("expected error for html input")
	}
	if _, _, err := ParseFeed([]byte(`<rss version="2.0"><channel><item>`)); err == nil {
		t.Fatal("expected error for malformed rss")
	}
	// 空 feed 不报错，返回空列表
	title, items, err := ParseFeed([]byte(`<rss version="2.0"><channel><title>空源</title></channel></rss>`))
	if err != nil {
		t.Fatalf("empty feed should not error: %v", err)
	}
	if title != "空源" || len(items) != 0 {
		t.Fatalf("unexpected empty feed result: %q %d", title, len(items))
	}
}

func TestParseFeedGBK(t *testing.T) {
	// "测试源" 的 GBK 字节
	gbkTitle := string([]byte{0xB2, 0xE2, 0xCA, 0xD4, 0xD4, 0xB4})
	feed := `<?xml version="1.0" encoding="GBK"?><rss version="2.0"><channel><title>` + gbkTitle + `</title><item><guid>g1</guid><title>第1话</title><link>https://e.com/1</link></item></channel></rss>`
	title, items, err := ParseFeed([]byte(feed))
	if err != nil {
		t.Fatalf("parse gbk: %v", err)
	}
	if title != "测试源" {
		t.Fatalf("gbk title: %q", title)
	}
	if len(items) != 1 || items[0].GUID != "g1" {
		t.Fatalf("gbk items: %+v", items)
	}
}
