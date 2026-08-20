package api

import (
	"net/http"
	"strings"

	"litepan/internal/medialibrary"
)

func (h *Handler) ensureMediaLibrary() *medialibrary.Service {
	if h.mediaLibrary == nil {
		return nil
	}
	return h.mediaLibrary
}

// mediaLibraryRoots 返回影视库根目录配置（公开/管理员）。
func (h *Handler) mediaLibraryRoots(w http.ResponseWriter, r *http.Request) {
	svc := h.ensureMediaLibrary()
	if !ensureServiceReady(w, svc != nil) {
		return
	}
	roots, err := svc.Roots(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, roots)
}

// mediaLibraryItems 返回影视条目列表（公开/管理员）。
func (h *Handler) mediaLibraryItems(w http.ResponseWriter, r *http.Request) {
	svc := h.ensureMediaLibrary()
	if !ensureServiceReady(w, svc != nil) {
		return
	}
	lib := strings.TrimSpace(r.URL.Query().Get("lib"))
	items, err := svc.Items(r.Context(), lib, parseStrmScrapeListQuery(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, items)
}

// mediaLibraryDetail 返回影视条目详情（公开/管理员）。
func (h *Handler) mediaLibraryDetail(w http.ResponseWriter, r *http.Request) {
	svc := h.ensureMediaLibrary()
	if !ensureServiceReady(w, svc != nil) {
		return
	}
	lib := strings.TrimSpace(r.URL.Query().Get("lib"))
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	detail, err := svc.Detail(r.Context(), lib, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, detail)
}

// mediaLibraryPoster 流式返回海报文件（公开/管理员）。lib 为不透明库 id，不泄露服务器路径。
func (h *Handler) mediaLibraryPoster(w http.ResponseWriter, r *http.Request) {
	svc := h.ensureMediaLibrary()
	if !ensureServiceReady(w, svc != nil) {
		return
	}
	lib := strings.TrimSpace(r.URL.Query().Get("lib"))
	rel := strings.TrimSpace(r.URL.Query().Get("rel"))
	path, err := svc.Poster(r.Context(), lib, rel)
	if err != nil {
		writeErr(w, err)
		return
	}
	http.ServeFile(w, r, path)
}

// mediaLibraryRefresh 强制重建索引并返回条目（公开/管理员）。
func (h *Handler) mediaLibraryRefresh(w http.ResponseWriter, r *http.Request) {
	svc := h.ensureMediaLibrary()
	if !ensureServiceReady(w, svc != nil) {
		return
	}
	lib := strings.TrimSpace(r.URL.Query().Get("lib"))
	items, err := svc.Refresh(r.Context(), lib, parseStrmScrapeListQuery(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, items)
}

// adminSaveMediaLibraryRoots 保存影视库根目录配置（管理员）。
func (h *Handler) adminSaveMediaLibraryRoots(w http.ResponseWriter, r *http.Request) {
	svc := h.ensureMediaLibrary()
	if !ensureServiceReady(w, svc != nil) {
		return
	}
	var roots []medialibrary.Root
	if err := decodeJSON(r, &roots); err != nil {
		writeErr(w, err)
		return
	}
	if err := svc.SaveRoots(r.Context(), roots); err != nil {
		writeErr(w, err)
		return
	}
	saved, err := svc.Roots(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, saved)
}
