package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"litepan/internal/domain"
	"litepan/internal/magnetfavorites"
)

type magnetFavoriteDTO struct {
	Hash      string `json:"hash"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Seeders   int    `json:"seeders"`
	Leechers  int    `json:"leechers"`
	Date      int64  `json:"date"`
	Category  string `json:"category,omitempty"`
	Magnet    string `json:"magnet"`
	ViewURL   string `json:"view_url,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

type addMagnetFavoriteReq struct {
	Hash     string `json:"hash"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Seeders  int    `json:"seeders"`
	Leechers int    `json:"leechers"`
	Date     int64  `json:"date"`
	Category string `json:"category"`
	Magnet   string `json:"magnet"`
	ViewURL  string `json:"view_url"`
}

func (h *Handler) listMagnetFavorites(w http.ResponseWriter, r *http.Request) {
	state, err := h.magnetFavorites.Get(r.Context())
	if err != nil {
		writeErr(w, domain.Errorf(domain.CodeInternal, "读取磁力收藏失败：%v", err))
		return
	}
	writeOK(w, magnetFavoriteStateToDTO(state))
}

func (h *Handler) addMagnetFavorite(w http.ResponseWriter, r *http.Request) {
	var req addMagnetFavoriteReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	state, err := h.magnetFavorites.Add(r.Context(), magnetfavorites.Item{
		Hash:     req.Hash,
		Name:     req.Name,
		Size:     req.Size,
		Seeders:  req.Seeders,
		Leechers: req.Leechers,
		Date:     req.Date,
		Category: req.Category,
		Magnet:   req.Magnet,
		ViewURL:  req.ViewURL,
	})
	if err != nil {
		writeErr(w, domain.Errorf(domain.CodeValidation, "%v", err))
		return
	}
	writeJSON(w, http.StatusOK, Resp{
		Success: true,
		Message: "已加入收藏",
		Data:    magnetFavoriteStateToDTO(state),
	})
}

func (h *Handler) removeMagnetFavorite(w http.ResponseWriter, r *http.Request) {
	hash := strings.TrimSpace(chi.URLParam(r, "hash"))
	if hash == "" {
		writeErr(w, domain.Errorf(domain.CodeValidation, "hash 不能为空"))
		return
	}
	state, err := h.magnetFavorites.Remove(r.Context(), hash)
	if err != nil {
		writeErr(w, domain.Errorf(domain.CodeInternal, "删除磁力收藏失败：%v", err))
		return
	}
	writeJSON(w, http.StatusOK, Resp{
		Success: true,
		Message: "已取消收藏",
		Data:    magnetFavoriteStateToDTO(state),
	})
}

func magnetFavoriteStateToDTO(state magnetfavorites.State) []magnetFavoriteDTO {
	out := make([]magnetFavoriteDTO, 0, len(state.Items))
	for _, item := range state.Items {
		out = append(out, magnetFavoriteDTO{
			Hash:      item.Hash,
			Name:      item.Name,
			Size:      item.Size,
			Seeders:   item.Seeders,
			Leechers:  item.Leechers,
			Date:      item.Date,
			Category:  item.Category,
			Magnet:    item.Magnet,
			ViewURL:   item.ViewURL,
			CreatedAt: item.CreatedAt,
		})
	}
	return out
}
