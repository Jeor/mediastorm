package handlers

import (
	"context"
	"net/http"

	"novastream/models"
	"novastream/services/metadata"
)

type curatedListOptionsService interface {
	GetCuratedListWithOptions(context.Context, []metadata.CuratedItem, string, metadata.ShelfLoadOptions) ([]models.TrendingItem, error)
}

type topTenCandidatesOptionsService interface {
	GetTopTenCandidatesWithOptions(context.Context, string, []string, metadata.ShelfLoadOptions) ([]models.TrendingItem, error)
}

func getCuratedListForRequest(r *http.Request, service metadataService, items []metadata.CuratedItem, label string) ([]models.TrendingItem, error) {
	if svc, ok := service.(curatedListOptionsService); ok {
		return svc.GetCuratedListWithOptions(r.Context(), items, label, parseShelfLoadOptions(r))
	}
	return service.GetCuratedList(r.Context(), items, label)
}
