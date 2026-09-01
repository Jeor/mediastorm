package handlers

import (
	"context"
	"strings"

	"novastream/services/homedesigner"
)

// homeDesignerCatalogCapabilities resolves live handler dependencies at request
// time because media-library services are installed after the UI handler is
// constructed during application startup.
type homeDesignerCatalogCapabilities struct{ handler *AdminUIHandler }

func (p homeDesignerCatalogCapabilities) CatalogCapabilities(ctx context.Context, actor homedesigner.Actor, scope homedesigner.Scope) homedesigner.CatalogCapabilities {
	capabilities := homedesigner.CatalogCapabilities{}
	h := p.handler
	if h == nil || h.configManager == nil {
		return capabilities
	}
	settings, err := h.configManager.Load()
	if err != nil {
		return capabilities
	}
	capabilities.BasePath = settings.Server.BasePath
	if !actor.IsAdmin {
		if h.usersService == nil {
			return capabilities
		}
		profile, found := h.usersService.Get(scope.ProfileID)
		if found && profile.AccountID == actor.AccountID {
			for _, account := range settings.Trakt.Accounts {
				if account.OwnerAccountID == actor.AccountID || (account.OwnerAccountID == "" && account.ID == profile.TraktAccountID) {
					capabilities.AuthorizedAccounts = append(capabilities.AuthorizedAccounts, homedesigner.CatalogAccountAuthorization{Provider: "trakt", AccountID: account.ID})
				}
			}
			for _, account := range settings.Simkl.Accounts {
				if account.OwnerAccountID == actor.AccountID || (account.OwnerAccountID == "" && account.ID == profile.SimklAccountID) {
					capabilities.AuthorizedAccounts = append(capabilities.AuthorizedAccounts, homedesigner.CatalogAccountAuthorization{Provider: "simkl", AccountID: account.ID})
				}
			}
		}
	}
	profileID := ""
	if scope.Kind == "profile" {
		profileID = scope.ProfileID
	}
	addLibrary := func(id, name string) {
		id, name = strings.TrimSpace(id), strings.TrimSpace(name)
		if id == "" {
			return
		}
		if h.libraryAccessService == nil {
			if !actor.IsAdmin {
				return
			}
		} else if allowed, err := h.libraryAccessService.CanAccess(ctx, id, actor.AccountID, profileID, actor.IsAdmin); err != nil || !allowed {
			return
		}
		capabilities.Libraries = append(capabilities.Libraries, homedesigner.CatalogLibrary{ID: id, Name: name})
	}
	if h.localMediaService != nil {
		if libraries, err := h.localMediaService.ListLibraries(ctx); err == nil {
			for _, library := range libraries {
				addLibrary(library.ID, library.Name)
			}
		}
	}
	if h.remoteMediaService != nil {
		if libraries, err := h.remoteMediaService.ListLibraries(ctx); err == nil {
			for _, library := range libraries {
				addLibrary(library.ID, library.Name)
			}
		}
	}
	return capabilities
}
