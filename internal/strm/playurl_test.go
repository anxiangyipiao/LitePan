package strm

import (
	"context"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"testing"

	"litepan/internal/settings"
	"litepan/internal/store"
)

func playURLTestService(t *testing.T) (*Service, *settings.Service, *store.Store) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, store.Options{Memory: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	st := store.New(db)
	settingsSvc, err := settings.New(ctx, st.Configs)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(ServiceOptions{
		Repo:     st.StrmTasks,
		Settings: settingsSvc,
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return svc, settingsSvc, st
}

// TestPlayURL 验证给 qB 等外部客户端生成的播放代理地址：
// 走 /api/strm/play + file_key 编码 + token 鉴权 + 可选签名。
func TestPlayURL(t *testing.T) {
	svc, _, _ := playURLTestService(t)
	ctx := context.Background()

	got, err := svc.PlayURL(ctx, 7, "file-abc-123", "Movie (2024).mkv")
	if err != nil {
		t.Fatalf("PlayURL 失败: %v", err)
	}
	if !strings.HasPrefix(got, "http://127.0.0.1:5211/api/strm/play/7/") {
		t.Fatalf("URL 前缀异常: %s", got)
	}
	if !strings.Contains(got, "/t/") {
		t.Fatalf("缺少 /t/ token 段: %s", got)
	}

	// token 应持久化且 MatchToken 能核验
	tokenPart := strings.SplitN(got, "/t/", 2)[1]
	token := strings.SplitN(tokenPart, "/n/", 2)[0]
	if token == "" {
		t.Fatalf("token 为空: %s", got)
	}
	ok, err := svc.MatchToken(ctx, token)
	if err != nil || !ok {
		t.Fatalf("MatchToken(%q) = %v/%v, want true", token, ok, err)
	}

	// 文件名应 URL 转义（空格/括号）且可还原
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("解析 URL 失败: %v", err)
	}
	if !strings.Contains(u.EscapedPath(), "/n/Movie%20%282024%29.mkv") {
		t.Fatalf("文件名未正确转义: %s", got)
	}

	// file_key 应能解码回原 file_id
	seg := strings.SplitN(u.Path, "/api/strm/play/", 2)[1]
	parts := strings.Split(seg, "/")
	decoded, err := DecodeFileKey(parts[1])
	if err != nil || decoded != "file-abc-123" {
		t.Fatalf("file_key 解码异常: %q, err=%v", decoded, err)
	}
}

// TestPlayURLSignatureEnabled 验证开启签名后 URL 带 /s/ 段。
func TestPlayURLSignatureEnabled(t *testing.T) {
	svc, settingsSvc, _ := playURLTestService(t)
	ctx := context.Background()

	if err := settingsSvc.Update(ctx, map[string]string{"strm_signature_enabled": "true"}); err != nil {
		t.Fatalf("开启签名失败: %v", err)
	}
	got, err := svc.PlayURL(ctx, 3, "sig-file", "demo.iso")
	if err != nil {
		t.Fatalf("PlayURL 失败: %v", err)
	}
	if !strings.Contains(got, "/s/") {
		t.Fatalf("签名开启后 URL 应含 /s/ 段: %s", got)
	}
}
