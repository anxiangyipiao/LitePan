package api

import (
	"net/http"
	"strconv"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/rss"
)

func (h *Handler) listRSSSubscriptions(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.rss != nil) {
		return
	}
	data, err := h.rss.ListSubscriptions(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) createRSSSubscription(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.rss != nil) {
		return
	}
	var in domain.RSSSubscription
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	data, err := h.rss.CreateSubscription(r.Context(), &in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) updateRSSSubscription(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.rss != nil) {
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var in domain.RSSSubscription
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	data, err := h.rss.UpdateSubscription(r.Context(), id, &in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) deleteRSSSubscription(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.rss != nil) {
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.rss.DeleteSubscription(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]any{"id": id})
}

func (h *Handler) toggleRSSSubscription(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.rss != nil) {
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	data, err := h.rss.ToggleSubscription(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) fetchRSSSubscription(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.rss != nil) {
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	data, err := h.rss.FetchNow(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) previewRSSFeed(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.rss != nil) {
		return
	}
	var in rss.PreviewInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	data, err := h.rss.PreviewFeed(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) listRSSHistory(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.rss != nil) {
		return
	}
	subscriptionID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("subscription_id")), 10, 64)
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	offset, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("offset")))
	data, err := h.rss.ListHistory(r.Context(), subscriptionID, limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) retryRSSHistory(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.rss != nil) {
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	data, err := h.rss.RetryHistory(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, data)
}

func (h *Handler) deleteRSSHistory(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.rss != nil) {
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.rss.DeleteHistory(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]any{"id": id})
}

func (h *Handler) clearRSSHistory(w http.ResponseWriter, r *http.Request) {
	if !ensureServiceReady(w, h.rss != nil) {
		return
	}
	subscriptionID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("subscription_id")), 10, 64)
	n, err := h.rss.ClearHistory(r.Context(), subscriptionID)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]any{"deleted": n})
}
