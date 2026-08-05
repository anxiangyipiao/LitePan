package api

import (
	"net/http"

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
