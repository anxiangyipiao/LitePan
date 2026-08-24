// Package rss 提供 RSS 2.0 / Atom 订阅源的抓取与解析，以及命中过滤所需的
// 集数提取与匹配引擎。供「自动追番/追剧」订阅服务使用。
package rss

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html/charset"

	"litepan/internal/domain"
)

const maxFeedBytes = 5 << 20

// FeedItem 是解析器产出的标准化条目，供匹配引擎与抓取流程使用。
type FeedItem struct {
	GUID       string
	Title      string
	Link       string
	PubDate    time.Time
	TorrentURL string
	InfoHash   string // 大写十六进制 btih；无 magnet 时为空
}

// ParseFeed 解析 RSS 2.0 或 Atom 源。返回源标题与标准化条目列表。
func ParseFeed(data []byte) (string, []FeedItem, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	root := detectRootElement(data)
	switch root {
	case "rss":
		return parseRSS(data)
	case "feed":
		return parseAtom(data)
	default:
		return "", nil, domain.Errorf(domain.CodeValidation, "不支持的订阅源格式（仅支持 RSS 2.0 / Atom）")
	}
}

func detectRootElement(data []byte) string {
	i := 0
	for {
		// 跳过前导空白与 BOM
		for i < len(data) && (data[i] == ' ' || data[i] == '\t' || data[i] == '\r' || data[i] == '\n') {
			i++
		}
		if i >= len(data) || data[i] != '<' {
			return ""
		}
		// 跳过 XML 声明与处理指令
		if i+1 < len(data) && data[i+1] == '?' {
			if j := bytes.Index(data[i:], []byte("?>")); j >= 0 {
				i += j + 2
				continue
			}
			return ""
		}
		// 跳过注释
		if i+1 < len(data) && bytes.HasPrefix(data[i:], []byte("<!--")) {
			if j := bytes.Index(data[i:], []byte("-->")); j >= 0 {
				i += j + 3
				continue
			}
			return ""
		}
		end := bytes.IndexByte(data[i:], '>')
		if end < 0 {
			return ""
		}
		name := data[i+1 : i+end]
		// 去掉可能的属性、命名空间前缀
		name = bytes.TrimLeft(name, "/")
		if k := bytes.IndexAny(name, " \t\r\n"); k >= 0 {
			name = name[:k]
		}
		name = bytes.ToLower(name)
		if k := bytes.IndexByte(name, ':'); k >= 0 {
			name = name[k+1:]
		}
		return string(name)
	}
}

func newXMLDecoder(data []byte) (*xml.Decoder, error) {
	// CharsetReader 处理 XML 声明里的 encoding（GBK/GB2312/Big5 等）。
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = charset.NewReaderLabel
	return dec, nil
}

// ---------- RSS 2.0 ----------

type rssFeed struct {
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	GUID        rssGUID   `xml:"guid"`
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	PubDate     string    `xml:"pubDate"`
	Enclosure   rssEncl   `xml:"enclosure"`
	MagnetURI   string    `xml:"magnetURI"` // nyaa/sukebei 的 torrent:magnetURI
	InfoHash    string    `xml:"infoHash"`  // nyaa/sukebei 的 nyaa:infoHash
	Description rawXML    `xml:"description"`
}

type rssGUID struct {
	Value string `xml:",chardata"`
}

type rssEncl struct {
	URL  string `xml:"url,attr"`
	Type string `xml:"type,attr"`
}

// ---------- Atom ----------

type atomFeed struct {
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID        string    `xml:"id"`
	Title     rawXML    `xml:"title"`
	Link      []atomLnk `xml:"link"`
	Published string    `xml:"published"`
	Updated   string    `xml:"updated"`
	Summary   rawXML    `xml:"summary"`
	Content   rawXML    `xml:"content"`
}

type atomLnk struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

// rawXML 捕获元素内全部原始内容（含子标签与属性值），用于从 HTML 描述里抽取 magnet。
type rawXML struct {
	Data string
}

func (r *rawXML) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var sb strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			sb.WriteString("<" + t.Name.Local)
			for _, a := range t.Attr {
				sb.WriteString(` ` + a.Name.Local + `="` + a.Value + `"`)
			}
			sb.WriteString(">")
		case xml.EndElement:
			depth--
			if depth > 0 {
				sb.WriteString("</" + t.Name.Local + ">")
			}
		case xml.CharData:
			sb.Write(t)
		case xml.Directive, xml.ProcInst, xml.Comment:
		}
	}
	r.Data = sb.String()
	return nil
}

func parseRSS(data []byte) (string, []FeedItem, error) {
	dec, err := newXMLDecoder(data)
	if err != nil {
		return "", nil, err
	}
	var feed rssFeed
	if err := dec.Decode(&feed); err != nil {
		return "", nil, fmt.Errorf("RSS 解析失败：%w", err)
	}
	items := make([]FeedItem, 0, len(feed.Channel.Items))
	for _, it := range feed.Channel.Items {
		torrentURL := pickTorrentURL(torrentSources{
			enclosureURL: it.Enclosure.URL,
			link:         it.Link,
			magnetURI:    it.MagnetURI,
			infoHash:     it.InfoHash,
			description:  it.Description.Data,
			title:        it.Title,
		})
		items = append(items, FeedItem{
			GUID:       itemGUID(it.GUID.Value, it.Link, it.Title, it.PubDate),
			Title:      strings.TrimSpace(it.Title),
			Link:       strings.TrimSpace(it.Link),
			PubDate:    parsePubDate(it.PubDate),
			TorrentURL: torrentURL,
			InfoHash:   infoHash(torrentURL),
		})
	}
	return strings.TrimSpace(feed.Channel.Title), items, nil
}

func parseAtom(data []byte) (string, []FeedItem, error) {
	dec, err := newXMLDecoder(data)
	if err != nil {
		return "", nil, err
	}
	var feed atomFeed
	if err := dec.Decode(&feed); err != nil {
		return "", nil, fmt.Errorf("Atom 解析失败：%w", err)
	}
	items := make([]FeedItem, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		altLink, encLink := atomLinks(e.Link)
		torrentURL := pickTorrentURL(torrentSources{
			enclosureURL: encLink,
			link:         altLink,
			description:  e.Summary.Data + e.Content.Data,
			title:        e.Title.Data,
		})
		date := strings.TrimSpace(e.Published)
		if date == "" {
			date = strings.TrimSpace(e.Updated)
		}
		items = append(items, FeedItem{
			GUID:       itemGUID(e.ID, altLink, e.Title.Data, date),
			Title:      strings.TrimSpace(e.Title.Data),
			Link:       altLink,
			PubDate:    parsePubDate(date),
			TorrentURL: torrentURL,
			InfoHash:   infoHash(torrentURL),
		})
	}
	return strings.TrimSpace(feed.Title), items, nil
}

// atomLinks 分离 alternate（详情页链接）与 enclosure（磁力/http 种子链接）。
func atomLinks(links []atomLnk) (alternate, enclosure string) {
	for _, l := range links {
		href := strings.TrimSpace(l.Href)
		rel := strings.ToLower(strings.TrimSpace(l.Rel))
		if rel == "enclosure" || strings.HasPrefix(strings.ToLower(href), "magnet:") {
			if enclosure == "" {
				enclosure = href
			}
			continue
		}
		if alternate == "" || rel == "alternate" {
			alternate = href
		}
	}
	return alternate, enclosure
}

// torrentSources 聚合从一条 feed 条目里抽取种子的各种候选来源。
type torrentSources struct {
	enclosureURL string // <enclosure url>
	link         string // <link>
	magnetURI    string // torrent:magnetURI（nyaa/sukebei）
	infoHash     string // nyaa:infoHash（仅 infohash 的源）
	description  string // <description>/<summary>/<content> 原文
	title        string // 用于构造磁力链的 dn 参数
}

// pickTorrentURL 按优先级选择可用的种子链接：
// enclosure 磁力 → 描述内嵌 magnet → torrent:magnetURI → link 磁力
// → 由 infohash 构造磁力链（覆盖仅 nyaa:infoHash 的源）→ enclosure/link http。
func pickTorrentURL(src torrentSources) string {
	enc := strings.TrimSpace(src.enclosureURL)
	lnk := strings.TrimSpace(src.link)
	if isMagnet(enc) {
		return enc
	}
	if m := findMagnetInText(src.description); m != "" {
		return m
	}
	if uri := strings.TrimSpace(src.magnetURI); isMagnet(uri) {
		return uri
	}
	if isMagnet(lnk) {
		return lnk
	}
	if m := buildMagnetFromHash(strings.TrimSpace(src.infoHash), src.title); m != "" {
		return m
	}
	if isTorrentHTTP(enc) {
		return enc
	}
	if isTorrentHTTP(lnk) {
		return lnk
	}
	return ""
}

// buildMagnetFromHash 由纯 infohash 构造磁力链（nyaa:infoHash 只有 40 位 hex 或 32 位 base32）。
func buildMagnetFromHash(hash, title string) string {
	if !validInfoHash(hash) {
		return ""
	}
	dn := strings.ReplaceAll(url.QueryEscape(truncateRunes(title, 200)), "+", "%20")
	return "magnet:?xt=urn:btih:" + hash + "&dn=" + dn
}

func validInfoHash(h string) bool {
	switch len(h) {
	case 40:
		_, err := hex.DecodeString(h)
		return err == nil
	case 32:
		for _, c := range h {
			if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(c >= '2' && c <= '7') {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// isHttpTorrentURL 判断是否为 http(s) 的 .torrent 文件链接。
func isHttpTorrentURL(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return false
	}
	return strings.HasSuffix(lower, ".torrent")
}

func isMagnet(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), "magnet:")
}

func isTorrentHTTP(s string) bool {
	lower := strings.ToLower(s)
	return (strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"))
}

var magnetInTextRe = regexp.MustCompile(`(?i)magnet:\?xt=urn:btih:[0-9a-fA-F]{32,40}`)

func findMagnetInText(s string) string {
	m := magnetInTextRe.FindString(s)
	if m == "" {
		return ""
	}
	// 截断到常见终止符，避免把后续属性拼进来
	if i := strings.IndexAny(m, `"'<>&`); i >= 0 {
		m = m[:i]
	}
	return m
}

func infoHash(magnet string) string {
	const prefix = "urn:btih:"
	i := strings.Index(magnet, prefix)
	if i < 0 {
		return ""
	}
	rest := magnet[i+len(prefix):]
	if j := strings.IndexAny(rest, "&?\""); j >= 0 {
		rest = rest[:j]
	}
	return strings.ToUpper(rest)
}

// itemGUID 保证非空：guid → link → sha1(title|pubdate) 兜底。
func itemGUID(guid, link, title, pubdate string) string {
	if v := strings.TrimSpace(guid); v != "" {
		return v
	}
	if v := strings.TrimSpace(link); v != "" {
		return v
	}
	sum := sha1.Sum([]byte(strings.TrimSpace(title) + "|" + strings.TrimSpace(pubdate)))
	return hex.EncodeToString(sum[:])
}

var pubDateLayouts = []string{
	time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC850, time.RFC3339,
	"2006-01-02 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04:05 MST",
	"2006-01-02",
}

func parsePubDate(raw string) time.Time {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range pubDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
