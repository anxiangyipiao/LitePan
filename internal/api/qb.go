package api

import (
	"net/http"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/qb"
)

func (h *Handler) getQBSettings(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.qb != nil) {
		return
	}
	cfg := h.qb.GetSettings()
	cfg.Password = "" // 屏蔽密码，不回传给前端
	writeOK(w, cfg)
}

func (h *Handler) updateQBSettings(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.qb != nil) {
		return
	}
	var req qb.Settings
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := h.qb.UpdateSettings(r.Context(), req); err != nil {
		writeErr(w, err)
		return
	}
	h.getQBSettings(w, r)
}

func (h *Handler) testQB(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.qb != nil) {
		return
	}
	result, err := h.qb.Test(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, result)
}

func (h *Handler) addQBDownload(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.qb != nil) {
		return
	}
	var req struct {
		URLs     []string `json:"urls"`
		SavePath string   `json:"save_path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if len(req.URLs) == 0 {
		writeErr(w, domain.Errorf(domain.CodeValidation, "urls 不能为空"))
		return
	}
	if err := h.qb.Add(r.Context(), req.URLs, req.SavePath); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (h *Handler) listQBDownloads(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.qb != nil) {
		return
	}
	tasks, err := h.qb.List(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, tasks)
}

func (h *Handler) deleteQBDownloads(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.qb != nil) {
		return
	}
	var req struct {
		Hashes      []string `json:"hashes"`
		DeleteFiles bool     `json:"delete_files"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if len(req.Hashes) == 0 {
		writeErr(w, domain.Errorf(domain.CodeValidation, "hashes 不能为空"))
		return
	}
	if err := h.qb.Delete(r.Context(), req.Hashes, req.DeleteFiles); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

// addFileToQBDownload 把文件浏览器里选中的文件直链发送到 qBittorrent 下载。
// .torrent 文件会以磁力/种子链接形式交给 qB 解析；普通文件按直链拉取。
func (h *Handler) addFileToQBDownload(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.qb != nil) || !ensureServiceReady(w, h.playback != nil) {
		return
	}
	var req struct {
		AccountID int64  `json:"account_id"`
		FileID    string `json:"file_id"`
		SavePath  string `json:"save_path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.AccountID <= 0 || strings.TrimSpace(req.FileID) == "" {
		writeErr(w, domain.Errorf(domain.CodeValidation, "参数不完整"))
		return
	}
	res, err := h.playback.Resolve(r.Context(), req.AccountID, req.FileID, "", false)
	if err != nil {
		writeErr(w, domain.Errorf(domain.CodeDriverError, "解析下载直链失败：%v", err))
		return
	}
	if res.File.IsDir {
		writeErr(w, domain.Errorf(domain.CodeValidation, "不能将文件夹发送到 qB"))
		return
	}
	url := strings.TrimSpace(res.Link.URL)
	if url == "" || !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		writeErr(w, domain.Errorf(domain.CodeValidation, "该文件不支持直链，无法发送到 qB，请改用代理下载"))
		return
	}
	if err := h.qb.Add(r.Context(), []string{url}, strings.TrimSpace(req.SavePath)); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]any{"ok": true, "name": res.File.Name})
}
