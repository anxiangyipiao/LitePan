package rss

import (
	"crypto/sha1"
	"encoding/hex"
	"testing"
)

func TestTorrentInfoHash(t *testing.T) {
	// info 字典（含自身闭合 e）
	info := []byte("d6:lengthi1000e4:name4:file12:piece lengthi16384e6:pieces20:0123456789abcdefghij")
	info = append(info, 'e')
	// 顶层字典：d + "4:info" + info + 根闭合 e
	torrent := append([]byte("d"), []byte("4:info")...)
	torrent = append(torrent, info...)
	torrent = append(torrent, 'e')

	got, err := torrentInfoHash(torrent)
	if err != nil {
		t.Fatalf("torrentInfoHash: %v", err)
	}
	want := sha1.Sum(info)
	if got != hex.EncodeToString(want[:]) {
		t.Fatalf("infohash = %s, want %s", got, hex.EncodeToString(want[:]))
	}
}

func TestTorrentInfoHashErrors(t *testing.T) {
	cases := [][]byte{
		nil,
		[]byte("not a torrent"),
		[]byte("d4:info"),                       // info 未闭合
		[]byte("d4:infoe"),                      // info 不是 dict
		[]byte("d8:announce5:hello4:infodx"),    // info 内非法字节
	}
	for _, c := range cases {
		if _, err := torrentInfoHash(c); err == nil {
			t.Errorf("expected error for %q", string(c))
		}
	}
}

func TestBuildMagnetFromHash(t *testing.T) {
	m := buildMagnetFromHash("ca118254d9785de4e6de9764c75193326b08b2b7", "某片 01")
	if m != "magnet:?xt=urn:btih:ca118254d9785de4e6de9764c75193326b08b2b7&dn=%E6%9F%90%E7%89%87%2001" {
		t.Fatalf("unexpected magnet: %q", m)
	}
	// 非法 hash 返回空
	if buildMagnetFromHash("xyz", "t") != "" {
		t.Fatal("invalid hash should yield empty magnet")
	}
	// base32 允许
	if buildMagnetFromHash("ABCDEFGHIJKLMNOPQRSTUVWXYZ234567", "t") == "" {
		t.Fatal("base32 hash should be accepted")
	}
}
