package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"litepan/internal/backup"
	"litepan/internal/domain"
)

func (h *Handler) listBackupJobs(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.backup != nil) {
		return
	}
	data, err := h.backup.ListJobs(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) createBackupJob(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.backup != nil) {
		return
	}
	var in domain.BackupJob
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	data, err := h.backup.CreateJob(r.Context(), &in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) updateBackupJob(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.backup != nil) {
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in domain.BackupJob
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	data, err := h.backup.UpdateJob(r.Context(), id, &in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) deleteBackupJob(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.backup != nil) {
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.backup.DeleteJob(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]any{"id": id})
}

func (h *Handler) toggleBackupJob(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.backup != nil) {
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	data, err := h.backup.ToggleJob(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) runBackupJob(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.backup != nil) {
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	data, err := h.backup.RunNow(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) listBackupRuns(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.backup != nil) {
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	data, err := h.backup.ListRuns(r.Context(), id, limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) backupRunStream(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.backup != nil) {
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.backup.GetJob(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	h.streamBackupNDJSON(w, r, func(emit func(backup.StreamEvent) error) error {
		return h.backup.StreamRunProgress(r.Context(), id, emit)
	})
}

func (h *Handler) streamBackupNDJSON(w http.ResponseWriter, r *http.Request, fn func(func(backup.StreamEvent) error) error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, domain.Errorf(domain.CodeInternal, "不支持流式响应"))
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	writeLine := func(event backup.StreamEvent) error {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "%s\n", raw); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := fn(writeLine); err != nil {
		_ = writeLine(backup.StreamEvent{"event": "error", "message": err.Error()})
	}
}
