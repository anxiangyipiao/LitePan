package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/qb"
	"litepan/internal/settings"
)

type qbAddRequest struct {
	Magnet   string `json:"magnet"`
	SavePath string `json:"savepath"`
	Category string `json:"category"`
}

func (h *Handler) qbAdd(w http.ResponseWriter, r *http.Request) {
	var in qbAddRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, domain.Errorf(domain.CodeValidation, "请求体格式错误"))
		return
	}
	magnet := strings.TrimSpace(in.Magnet)
	if magnet == "" {
		writeErr(w, domain.Errorf(domain.CodeValidation, "磁力链不能为空"))
		return
	}
	savePath := strings.TrimSpace(in.SavePath)
	if savePath == "" {
		savePath = strings.TrimSpace(h.settingString(settings.KeyQBSavePath))
	}
	category := strings.TrimSpace(in.Category)
	if category == "" {
		category = strings.TrimSpace(h.settingString(settings.KeyQBCategory))
	}
	cl := qb.NewClient(qb.Options{
		BaseURL:  h.settingString(settings.KeyQBURL),
		Username: h.settingString(settings.KeyQBUsername),
		Password: h.settingString(settings.KeyQBPassword),
	})
	if err := cl.AddMagnet(r.Context(), magnet, savePath, category); err != nil {
		writeErr(w, domain.Errorf(domain.CodeDriverError, "%v", err))
		return
	}
	writeOK(w, map[string]any{"ok": true})
}

func (h *Handler) qbTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL      string `json:"url"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	baseURL := strings.TrimSpace(in.URL)
	username := strings.TrimSpace(in.Username)
	password := in.Password
	hasOverride := baseURL != "" || username != "" || in.Password != ""
	if !hasOverride {
		baseURL = h.settingString(settings.KeyQBURL)
		username = h.settingString(settings.KeyQBUsername)
		password = h.settingString(settings.KeyQBPassword)
	}
	if baseURL == "" {
		baseURL = h.settingString(settings.KeyQBURL)
	}
	cl := qb.NewClient(qb.Options{
		BaseURL:  baseURL,
		Username: username,
		Password: password,
	})
	if err := cl.Test(r.Context()); err != nil {
		writeErr(w, domain.Errorf(domain.CodeDriverError, "%v", err))
		return
	}
	writeOK(w, map[string]any{"ok": true})
}
