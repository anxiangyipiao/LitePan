package qb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestClientFlow 用 mock qB 服务器验证：登录 → 保持 SID Cookie → 取任务列表。
func TestClientFlow(t *testing.T) {
	var sid string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse form: %v", err)
			}
			if r.PostForm.Get("username") == "admin" && r.PostForm.Get("password") == "pass" {
				sid = "SID-123"
				http.SetCookie(w, &http.Cookie{Name: "SID", Value: sid, Path: "/"})
				_, _ = w.Write([]byte("Ok."))
				return
			}
			_, _ = w.Write([]byte("Fails."))
		case "/api/v2/app/version":
			_, _ = w.Write([]byte("v5.1.4"))
		case "/api/v2/torrents/info":
			// 校验 SID Cookie
			c, err := r.Cookie("SID")
			if err != nil || c.Value != sid {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"bad sid"}`))
				return
			}
			_, _ = w.Write([]byte(`[{"hash":"abc","name":"test.torrent","state":"downloading","progress":0.5,"size":1024,"save_path":"/d","added_on":1}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL, Username: "admin", Password: "pass"})

	version, err := c.Test(context.Background())
	if err != nil {
		t.Fatalf("Test 失败: %v", err)
	}
	if version != "v5.1.4" {
		t.Fatalf("version = %q", version)
	}

	tasks, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Hash != "abc" || tasks[0].Progress != 50 || tasks[0].State != "running" {
		t.Fatalf("List 结果异常: %+v", tasks)
	}
}

// TestClientLoginFailure 验证密码错误时的提示。
func TestClientLoginFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Fails."))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL, Username: "admin", Password: "wrong"})
	_, err := c.Test(context.Background())
	if err == nil || !strings.Contains(err.Error(), "登录失败") {
		t.Fatalf("应报登录失败，实际 err=%v", err)
	}
}

// TestClient404ShowsURL 验证 404 时错误信息带上完整 URL。
func TestClient404ShowsURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL, Username: "a", Password: "b"})
	_, err := c.Test(context.Background())
	if err == nil || !strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("404 错误应包含完整 URL，实际 err=%v", err)
	}
}

func TestNormalizeState(t *testing.T) {
	cases := map[string]string{
		"downloading":  "running",
		"forcedDL":     "running",
		"metaDL":       "running",
		"stalledDL":    "running",
		"uploading":    "seeding",
		"forcedUP":     "seeding",
		"pausedDL":     "paused",
		"pausedUP":     "paused",
		"error":        "error",
		"missingFiles": "error",
		"completed":    "finished",
		"unknownState": "running",
	}
	for in, want := range cases {
		if got := normalizeState(in); got != want {
			t.Fatalf("normalizeState(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJoinURLs(t *testing.T) {
	if got := joinURLs([]string{" magnet:1 ", "", " magnet:2"}); got != "magnet:1\nmagnet:2" {
		t.Fatalf("joinURLs = %q", got)
	}
}
