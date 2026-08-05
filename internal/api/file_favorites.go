package api

import (
	"net/http"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/favorites"
)

type favoriteCrumbDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type favoriteItemDTO struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	AccountID int64              `json:"account_id"`
	Crumbs    []favoriteCrumbDTO `json:"crumbs"`
}

type favoritesStateDTO struct {
	Open  bool              `json:"open"`
	Items []favoriteItemDTO `json:"items"`
}

type saveFavoritesReq struct {
	Open  bool              `json:"open"`
	Items []favoriteItemDTO `json:"items"`
}

func (h *Handler) getFavorites(w http.ResponseWriter, r *http.Request) {
	state, err := h.favorites.Get(r.Context())
	if err != nil {
		writeErr(w, domain.Errorf(domain.CodeInternal, "读取收藏夹失败"))
		return
	}
	writeOK(w, favoriteStateToDTO(state))
}

func (h *Handler) saveFavorites(w http.ResponseWriter, r *http.Request) {
	var req saveFavoritesReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, err)
		return
	}
	state, err := h.favorites.Put(r.Context(), favorites.State{
		Open:  req.Open,
		Items: favoriteItemsFromDTO(req.Items),
	})
	if err != nil {
		writeErr(w, domain.Errorf(domain.CodeInternal, "保存收藏夹失败"))
		return
	}
	writeJSON(w, http.StatusOK, Resp{
		Success: true,
		Message: "收藏夹保存成功",
		Data:    favoriteStateToDTO(state),
	})
}

func favoriteStateToDTO(state favorites.State) favoritesStateDTO {
	out := favoritesStateDTO{
		Open:  state.Open,
		Items: make([]favoriteItemDTO, 0, len(state.Items)),
	}
	for _, item := range state.Items {
		dto := favoriteItemDTO{
			ID:        item.ID,
			Name:      item.Name,
			AccountID: item.AccountID,
			Crumbs:    make([]favoriteCrumbDTO, 0, len(item.Crumbs)),
		}
		for _, crumb := range item.Crumbs {
			dto.Crumbs = append(dto.Crumbs, favoriteCrumbDTO{
				ID:   crumb.ID,
				Name: crumb.Name,
			})
		}
		out.Items = append(out.Items, dto)
	}
	return out
}

func favoriteItemsFromDTO(items []favoriteItemDTO) []favorites.Item {
	out := make([]favorites.Item, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		name := strings.TrimSpace(item.Name)
		if id == "" || name == "" {
			continue
		}
		next := favorites.Item{
			ID:        id,
			Name:      name,
			AccountID: item.AccountID,
			Crumbs:    make([]favorites.Crumb, 0, len(item.Crumbs)),
		}
		for _, crumb := range item.Crumbs {
			crumbName := strings.TrimSpace(crumb.Name)
			if crumbName == "" {
				continue
			}
			next.Crumbs = append(next.Crumbs, favorites.Crumb{
				ID:   strings.TrimSpace(crumb.ID),
				Name: crumbName,
			})
		}
		out = append(out, next)
	}
	return out
}
