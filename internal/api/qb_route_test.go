package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"litepan/internal/upload"
)

// TestQBRoutesRegistered 验证 qB 管理端点已注册在 /api/admin/qb/*（前端 qb.ts 调用的路径）。
// 空依赖下 requireAdmin 会 panic，被 Recoverer 转成 500；只要不是 404 就说明路由已命中。
func TestQBRoutesRegistered(t *testing.T) {
	router := NewRouter(Deps{Uploads: upload.NewManager(upload.Options{})})

	cases := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/admin/qb/settings"},
		{http.MethodPut, "/api/admin/qb/settings"},
		{http.MethodPost, "/api/admin/qb/test"},
		{http.MethodPost, "/api/admin/qb/add"},
		{http.MethodPost, "/api/admin/qb/add-file"},
		{http.MethodGet, "/api/admin/qb/tasks"},
		{http.MethodPost, "/api/admin/qb/tasks/delete"},
	}
	for _, c := range cases {
		req := httptest.NewRequest(c.method, c.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("%s %s → 404：qB 路由未注册在 /api/admin/qb/*", c.method, c.path)
		}
		t.Logf("%s %s → %d（非 404，路由已命中）", c.method, c.path, rec.Code)
	}
}
