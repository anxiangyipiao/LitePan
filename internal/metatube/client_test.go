package metatube

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDedupeSearchItemsKeepsBestScorePerNumber(t *testing.T) {
	items := []map[string]any{
		{"id": "SSIS-123", "number": "SSIS-123", "provider": "JavBus", "score": 0},
		{"id": "ssis00123", "number": "SSIS-123", "provider": "JAV321", "score": 5},
		{"id": "OTHER-1", "number": "OTHER-1", "provider": "JavBus", "score": 3},
	}
	got := dedupeSearchItems(items)
	if len(got) != 2 {
		t.Fatalf("dedup len=%d, want 2", len(got))
	}
	if got[0]["provider"] != "JAV321" {
		t.Fatalf("最高分 provider 应排最前，got %v", got[0]["provider"])
	}
	if got[1]["number"] != "OTHER-1" {
		t.Fatalf("第二个命中应为 OTHER-1，got %v", got[1]["number"])
	}
}

func TestSearchItemToTMDBShapeUsesNumberAsMatchKey(t *testing.T) {
	out := searchItemToTMDBShape(map[string]any{
		"id":           "ssis00123",
		"number":       "SSIS-123",
		"provider":     "JAV321",
		"title":        "無自覚なフリして",
		"release_date": "2021-07-19T00:00:00Z",
		"thumb_url":    "https://example.com/t.jpg",
		"actors":       []any{"乙白さやか"},
	})
	if out["id"] != "SSIS-123" || out["original_title"] != "SSIS-123" {
		t.Fatalf("id/original_title 应为番号，got id=%v original=%v", out["id"], out["original_title"])
	}
	if out["_metatube_id"] != "ssis00123" || out["_metatube_provider"] != "JAV321" {
		t.Fatalf("provider/id 翻译错误：%v", out)
	}
	if out["title"] != "SSIS-123 無自覚なフリして" {
		t.Fatalf("搜索命中标题应带番号前缀，got %v", out["title"])
	}
	if out["poster_path"] != "https://example.com/t.jpg" {
		t.Fatalf("搜索命中 poster_path 应为缩略图 URL，got %v", out["poster_path"])
	}
	actors, _ := out["actors"].([]string)
	if len(actors) != 1 || actors[0] != "乙白さやか" {
		t.Fatalf("actors 翻译错误：%v", actors)
	}
}

func TestDetailToTMDBShapeMergesRichFields(t *testing.T) {
	hit := searchItemToTMDBShape(map[string]any{
		"id": "ssis00123", "number": "SSIS-123", "provider": "JAV321",
		"title": "無自覚なフリして", "thumb_url": "https://example.com/t.jpg",
	})
	out := detailToTMDBShape(hit, map[string]any{
		"id":           "ssis00123",
		"number":       "SSIS-123",
		"provider":     "JAV321",
		"title":        "無自覚なフリして誘惑",
		"summary":      "这是一个简介",
		"release_date": "2021-07-19T00:00:00Z",
		"runtime":      148,
		"maker":        "S1 NO.1",
		"director":     "大崎広浩治",
		"genres":       []any{"美少女", "単体作品"},
		"actors":       []any{"乙白さやか"},
	})
	if out["overview"] != "这是一个简介" {
		t.Fatalf("overview 未合并，got %v", out["overview"])
	}
	if out["title"] != "SSIS-123 無自覚なフリして誘惑" {
		t.Fatalf("详情标题应带番号前缀，got %v", out["title"])
	}
	if out["poster_path"] != "JAV321/ssis00123" {
		t.Fatalf("详情 poster_path 应为 provider/id 供图片端点下载，got %v", out["poster_path"])
	}
	if out["studio"] != "S1 NO.1" {
		t.Fatalf("studio 未取 maker，got %v", out["studio"])
	}
	if out["runtime"] != 148 {
		t.Fatalf("runtime 未合并，got %v", out["runtime"])
	}
	genres, _ := out["genres"].([]string)
	if len(genres) != 2 {
		t.Fatalf("genres 未合并，got %v", out["genres"])
	}
}

func TestStringListFieldHandlesBothShapes(t *testing.T) {
	if got := stringListField([]any{"a", "b"}); len(got) != 2 {
		t.Fatalf("string 数组形状失败：%v", got)
	}
	if got := stringListField([]any{map[string]any{"name": "x"}, map[string]any{"name": "y"}}); len(got) != 2 {
		t.Fatalf("[{name}] 形状失败：%v", got)
	}
	if got := stringListField(nil); got != nil {
		t.Fatalf("nil 应返回 nil，got %v", got)
	}
}

func TestSearchHandlesInfoNotFoundAsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"info not found"}}`))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	results, err := c.Search(context.Background(), "zzz", nil, "movie")
	if err != nil {
		t.Fatalf("info not found 应视为空结果而非错误：%v", err)
	}
	if len(results) != 0 {
		t.Fatalf("应返回空结果，got %d", len(results))
	}
}

func TestSearchParsesDataHits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"SSIS-123","number":"SSIS-123","provider":"JavBus","title":"T1","release_date":"2021-07-14T00:00:00Z","thumb_url":"http://t/1.jpg","score":0}]}`))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	results, err := c.Search(context.Background(), "SSIS-123", nil, "movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	var m map[string]any
	if err := json.Unmarshal(results[0], &m); err != nil {
		t.Fatal(err)
	}
	if m["id"] != "SSIS-123" || m["original_title"] != "SSIS-123" {
		t.Fatalf("命中未翻译成番号键：%v", m)
	}
}

func TestValidateConnectionRootOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"app":"metatube","version":"v1.4.0"}}`))
	}))
	defer srv.Close()

	c := NewClient(Options{BaseURL: srv.URL})
	if !c.ValidateConnection(context.Background()) {
		t.Fatal("根路径探活应成功")
	}
	if !strings.HasPrefix(c.baseURL, "http") {
		t.Fatalf("baseURL 未规范化：%q", c.baseURL)
	}
}
