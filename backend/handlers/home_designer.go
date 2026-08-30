package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"novastream/services/homedesigner"
)

type homeDesignerErrorResponse struct {
	Code    string                    `json:"code"`
	Message string                    `json:"message"`
	Fields  []homedesigner.FieldError `json:"fields,omitempty"`
}

// HomeDesignerPage renders the authenticated editor shell. Controls and
// interactions are deliberately loaded by the module asset rather than this
// server-rendered boundary.
func (h *AdminUIHandler) HomeDesignerPage(w http.ResponseWriter, r *http.Request) {
	isAdmin, accountID, basePath, username := h.getPageRoleInfo(r)
	users := h.getScopedUsers(isAdmin, accountID)
	data := AdminPageData{
		CurrentPath:    basePath + "/settings/home-designer",
		BasePath:       basePath,
		ServerBasePath: h.serverBasePath,
		IsAdmin:        isAdmin,
		AccountID:      accountID,
		Username:       username,
		Users:          users,
		Version:        GetBackendVersion(),
		BuildID:        GetBackendBuildID(),
		NoProfiles:     !isAdmin && len(users) == 0,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.homeDesignerTemplate.ExecuteTemplate(w, "base", data); err != nil {
		fmt.Printf("Home Designer template error: %v\n", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// GetHomeDesigner returns an authorized Home Designer document for the
// requested global or profile scope.
func (h *AdminUIHandler) GetHomeDesigner(w http.ResponseWriter, r *http.Request) {
	scope, ok := homeDesignerScopeFromQuery(w, r)
	if !ok {
		return
	}
	document, err := h.homeDesignerService.Load(r.Context(), h.homeDesignerActor(r), scope)
	if err != nil {
		homeDesignerServiceError(w, err)
		return
	}
	homeDesignerJSON(w, http.StatusOK, document)
}

// PutHomeDesigner applies an explicitly submitted, revision-checked Home
// Designer document update. It never writes on page load or preview requests.
func (h *AdminUIHandler) PutHomeDesigner(w http.ResponseWriter, r *http.Request) {
	request, err := decodeHomeDesignerApply(w, r)
	if err != nil {
		return
	}
	document, err := h.homeDesignerService.Apply(r.Context(), h.homeDesignerActor(r), request)
	if err != nil {
		homeDesignerServiceError(w, err)
		return
	}
	homeDesignerJSON(w, http.StatusOK, document)
}

func (h *AdminUIHandler) homeDesignerActor(r *http.Request) homedesigner.Actor {
	isAdmin, accountID, _, _ := h.getPageRoleInfo(r)
	return homedesigner.Actor{IsAdmin: isAdmin, AccountID: accountID}
}

func homeDesignerScopeFromQuery(w http.ResponseWriter, r *http.Request) (homedesigner.Scope, bool) {
	scope := homedesigner.Scope{Kind: strings.TrimSpace(r.URL.Query().Get("scope")), ProfileID: strings.TrimSpace(r.URL.Query().Get("profileId"))}
	if scope.Kind == "" {
		homeDesignerError(w, http.StatusUnprocessableEntity, "validation_error", "scope is required", []homedesigner.FieldError{{Section: "scope", Path: "scope", Message: "scope is required"}})
		return homedesigner.Scope{}, false
	}
	return scope, true
}

func decodeHomeDesignerApply(w http.ResponseWriter, r *http.Request) (homedesigner.ApplyRequest, error) {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var request homedesigner.ApplyRequest
	if err := decoder.Decode(&request); err != nil {
		homeDesignerError(w, http.StatusBadRequest, "invalid_request", "request body must be a valid Home Designer update", nil)
		return homedesigner.ApplyRequest{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		homeDesignerError(w, http.StatusBadRequest, "invalid_request", "request body must contain one JSON document", nil)
		if err == nil {
			err = errors.New("multiple JSON documents")
		}
		return homedesigner.ApplyRequest{}, err
	}
	return request, nil
}

func homeDesignerServiceError(w http.ResponseWriter, err error) {
	var validation homedesigner.ValidationError
	switch {
	case errors.Is(err, homedesigner.ErrForbidden):
		homeDesignerError(w, http.StatusForbidden, "forbidden", "Home Designer access is forbidden", nil)
	case errors.Is(err, homedesigner.ErrProfileNotFound):
		homeDesignerError(w, http.StatusNotFound, "not_found", "Home Designer profile was not found", nil)
	case errors.As(err, &validation):
		homeDesignerError(w, http.StatusUnprocessableEntity, "validation_error", validation.Error(), validation.Fields)
	case errors.Is(err, homedesigner.ErrRevisionConflict):
		homeDesignerError(w, http.StatusConflict, "revision_conflict", "Home Designer changed before this update", nil)
	default:
		homeDesignerError(w, http.StatusInternalServerError, "internal_error", "Home Designer could not be completed", nil)
	}
}

func homeDesignerError(w http.ResponseWriter, status int, code, message string, fields []homedesigner.FieldError) {
	homeDesignerJSON(w, status, homeDesignerErrorResponse{Code: code, Message: message, Fields: fields})
}

func homeDesignerJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
