package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"

	"novastream/config"
)

const (
	adminDashboardLayoutVersion = 1
	adminDashboardColumns       = 12
	adminDashboardMaxBodyBytes  = 32 << 10
)

type adminDashboardModuleDefinition struct {
	ID       string `json:"id"`
	MinW     int    `json:"minW"`
	MinH     int    `json:"minH"`
	MaxW     int    `json:"maxW"`
	MaxH     int    `json:"maxH"`
	Advanced bool   `json:"advanced,omitempty"`
	DefaultX int    `json:"defaultX"`
	DefaultY int    `json:"defaultY"`
	DefaultW int    `json:"defaultW"`
	DefaultH int    `json:"defaultH"`
}

type adminDashboardLayoutResponse struct {
	Version     int                                 `json:"version"`
	Columns     int                                 `json:"columns"`
	Modules     []config.AdminDashboardLayoutModule `json:"modules"`
	Definitions []adminDashboardModuleDefinition    `json:"definitions"`
}

var adminDashboardModules = []adminDashboardModuleDefinition{
	{ID: "system-status", MinW: 2, MinH: 2, MaxW: 6, MaxH: 3, DefaultX: 0, DefaultY: 0, DefaultW: 3, DefaultH: 2},
	{ID: "active-stream-count", MinW: 2, MinH: 2, MaxW: 6, MaxH: 3, DefaultX: 3, DefaultY: 0, DefaultW: 3, DefaultH: 2},
	{ID: "backend-uptime", MinW: 2, MinH: 2, MaxW: 6, MaxH: 3, DefaultX: 6, DefaultY: 0, DefaultW: 3, DefaultH: 2},
	{ID: "provider-readiness", MinW: 2, MinH: 2, MaxW: 6, MaxH: 3, DefaultX: 9, DefaultY: 0, DefaultW: 3, DefaultH: 2},
	{ID: "active-streams", MinW: 6, MinH: 5, MaxW: 12, MaxH: 12, DefaultX: 0, DefaultY: 2, DefaultW: 8, DefaultH: 7},
	{ID: "provider-health", MinW: 3, MinH: 5, MaxW: 8, MaxH: 10, DefaultX: 8, DefaultY: 2, DefaultW: 4, DefaultH: 7},
	{ID: "watch-time", MinW: 5, MinH: 5, MaxW: 12, MaxH: 10, DefaultX: 0, DefaultY: 9, DefaultW: 7, DefaultH: 6},
	{ID: "recently-watched", MinW: 4, MinH: 5, MaxW: 8, MaxH: 10, DefaultX: 7, DefaultY: 9, DefaultW: 5, DefaultH: 6},
	{ID: "usenet-connections", MinW: 8, MinH: 4, MaxW: 12, MaxH: 10, Advanced: true, DefaultX: 0, DefaultY: 15, DefaultW: 12, DefaultH: 6},
	{ID: "live-stream-limits", MinW: 8, MinH: 4, MaxW: 12, MaxH: 10, Advanced: true, DefaultX: 0, DefaultY: 21, DefaultW: 12, DefaultH: 6},
	{ID: "vod-stream-limits", MinW: 8, MinH: 4, MaxW: 12, MaxH: 10, Advanced: true, DefaultX: 0, DefaultY: 27, DefaultW: 12, DefaultH: 6},
	{ID: "usenet-providers", MinW: 4, MinH: 5, MaxW: 8, MaxH: 10, Advanced: true, DefaultX: 0, DefaultY: 33, DefaultW: 6, DefaultH: 7},
	{ID: "debrid-providers", MinW: 4, MinH: 5, MaxW: 8, MaxH: 10, Advanced: true, DefaultX: 6, DefaultY: 33, DefaultW: 6, DefaultH: 7},
	{ID: "endpoint-health", MinW: 8, MinH: 4, MaxW: 12, MaxH: 10, Advanced: true, DefaultX: 0, DefaultY: 40, DefaultW: 12, DefaultH: 6},
	{ID: "configuration-summary", MinW: 8, MinH: 6, MaxW: 12, MaxH: 12, Advanced: true, DefaultX: 0, DefaultY: 46, DefaultW: 12, DefaultH: 8},
	{ID: "indexers", MinW: 4, MinH: 5, MaxW: 8, MaxH: 10, Advanced: true, DefaultX: 0, DefaultY: 54, DefaultW: 6, DefaultH: 7},
	{ID: "torrent-scrapers", MinW: 4, MinH: 5, MaxW: 8, MaxH: 10, Advanced: true, DefaultX: 6, DefaultY: 54, DefaultW: 6, DefaultH: 7},
}

var adminDashboardLayoutMu sync.Mutex

func defaultAdminDashboardLayout() config.AdminDashboardLayout {
	modules := make([]config.AdminDashboardLayoutModule, 0, len(adminDashboardModules))
	for _, definition := range adminDashboardModules {
		modules = append(modules, config.AdminDashboardLayoutModule{
			ID: definition.ID,
			X:  definition.DefaultX,
			Y:  definition.DefaultY,
			W:  definition.DefaultW,
			H:  definition.DefaultH,
		})
	}
	return config.AdminDashboardLayout{Version: adminDashboardLayoutVersion, Modules: modules}
}

func adminDashboardDefinitionMap() map[string]adminDashboardModuleDefinition {
	definitions := make(map[string]adminDashboardModuleDefinition, len(adminDashboardModules))
	for _, definition := range adminDashboardModules {
		definitions[definition.ID] = definition
	}
	return definitions
}

func validateAdminDashboardLayout(layout config.AdminDashboardLayout) (config.AdminDashboardLayout, error) {
	if layout.Version != adminDashboardLayoutVersion {
		return config.AdminDashboardLayout{}, fmt.Errorf("unsupported dashboard layout version %d", layout.Version)
	}
	if len(layout.Modules) != len(adminDashboardModules) {
		return config.AdminDashboardLayout{}, fmt.Errorf("dashboard layout must contain exactly %d modules", len(adminDashboardModules))
	}

	definitions := adminDashboardDefinitionMap()
	seen := make(map[string]bool, len(layout.Modules))
	modules := append([]config.AdminDashboardLayoutModule(nil), layout.Modules...)
	for _, module := range modules {
		definition, ok := definitions[module.ID]
		if !ok {
			return config.AdminDashboardLayout{}, fmt.Errorf("unknown dashboard module %q", module.ID)
		}
		if seen[module.ID] {
			return config.AdminDashboardLayout{}, fmt.Errorf("dashboard module %q appears more than once", module.ID)
		}
		seen[module.ID] = true
		if err := validateAdminDashboardModule(module, definition); err != nil {
			return config.AdminDashboardLayout{}, err
		}
	}
	if dashboardModulesOverlap(modules) {
		return config.AdminDashboardLayout{}, errors.New("dashboard modules must not overlap")
	}

	compactAdminDashboardModules(modules)
	return config.AdminDashboardLayout{Version: adminDashboardLayoutVersion, Modules: modules}, nil
}

func validateAdminDashboardModule(module config.AdminDashboardLayoutModule, definition adminDashboardModuleDefinition) error {
	if module.X < 0 || module.Y < 0 || module.X+module.W > adminDashboardColumns {
		return fmt.Errorf("dashboard module %q is outside the %d-column grid", module.ID, adminDashboardColumns)
	}
	if module.W < definition.MinW || module.W > definition.MaxW || module.H < definition.MinH || module.H > definition.MaxH {
		return fmt.Errorf("dashboard module %q has unsupported dimensions %dx%d", module.ID, module.W, module.H)
	}
	return nil
}

func dashboardModulesOverlap(modules []config.AdminDashboardLayoutModule) bool {
	for i := range modules {
		for j := i + 1; j < len(modules); j++ {
			if dashboardModulesIntersect(modules[i], modules[j]) {
				return true
			}
		}
	}
	return false
}

func dashboardModulesIntersect(a, b config.AdminDashboardLayoutModule) bool {
	return a.X < b.X+b.W && a.X+a.W > b.X && a.Y < b.Y+b.H && a.Y+a.H > b.Y
}

func compactAdminDashboardModules(modules []config.AdminDashboardLayoutModule) {
	order := make([]int, len(modules))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := modules[order[i]], modules[order[j]]
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		return a.X < b.X
	})

	placed := make([]config.AdminDashboardLayoutModule, 0, len(modules))
	for _, index := range order {
		module := modules[index]
		for module.Y > 0 {
			candidate := module
			candidate.Y--
			blocked := false
			for _, other := range placed {
				if dashboardModulesIntersect(candidate, other) {
					blocked = true
					break
				}
			}
			if blocked {
				break
			}
			module = candidate
		}
		modules[index] = module
		placed = append(placed, module)
	}
}

func normalizedStoredAdminDashboardLayout(stored *config.AdminDashboardLayout) config.AdminDashboardLayout {
	if stored == nil {
		return defaultAdminDashboardLayout()
	}
	if normalized, err := validateAdminDashboardLayout(*stored); err == nil {
		return normalized
	}

	definitions := adminDashboardDefinitionMap()
	seen := make(map[string]bool, len(stored.Modules))
	modules := make([]config.AdminDashboardLayoutModule, 0, len(adminDashboardModules))
	maxBottom := 0
	for _, module := range stored.Modules {
		definition, ok := definitions[module.ID]
		if !ok || seen[module.ID] || validateAdminDashboardModule(module, definition) != nil {
			continue
		}
		seen[module.ID] = true
		modules = append(modules, module)
		if bottom := module.Y + module.H; bottom > maxBottom {
			maxBottom = bottom
		}
	}
	if dashboardModulesOverlap(modules) {
		return defaultAdminDashboardLayout()
	}

	firstMissingY := -1
	for _, definition := range adminDashboardModules {
		if !seen[definition.ID] && (firstMissingY < 0 || definition.DefaultY < firstMissingY) {
			firstMissingY = definition.DefaultY
		}
	}
	for _, definition := range adminDashboardModules {
		if seen[definition.ID] {
			continue
		}
		modules = append(modules, config.AdminDashboardLayoutModule{
			ID: definition.ID,
			X:  definition.DefaultX,
			Y:  maxBottom + definition.DefaultY - firstMissingY,
			W:  definition.DefaultW,
			H:  definition.DefaultH,
		})
	}
	compactAdminDashboardModules(modules)
	return config.AdminDashboardLayout{Version: adminDashboardLayoutVersion, Modules: modules}
}

func dashboardLayoutResponse(layout config.AdminDashboardLayout) adminDashboardLayoutResponse {
	return adminDashboardLayoutResponse{
		Version:     layout.Version,
		Columns:     adminDashboardColumns,
		Modules:     layout.Modules,
		Definitions: append([]adminDashboardModuleDefinition(nil), adminDashboardModules...),
	}
}

// GetDashboardLayout returns the shared, normalized administrator dashboard layout.
func (h *AdminUIHandler) GetDashboardLayout(w http.ResponseWriter, _ *http.Request) {
	if h.configManager == nil {
		http.Error(w, "Configuration manager not available", http.StatusInternalServerError)
		return
	}
	settings, err := h.configManager.Load()
	if err != nil {
		http.Error(w, "Failed to load dashboard layout", http.StatusInternalServerError)
		return
	}
	writeDashboardLayoutJSON(w, http.StatusOK, normalizedStoredAdminDashboardLayout(settings.UI.AdminDashboardLayout))
}

// SaveDashboardLayout validates and stores the shared administrator dashboard layout.
func (h *AdminUIHandler) SaveDashboardLayout(w http.ResponseWriter, r *http.Request) {
	if h.configManager == nil {
		http.Error(w, "Configuration manager not available", http.StatusInternalServerError)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, adminDashboardMaxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var submitted config.AdminDashboardLayout
	if err := decoder.Decode(&submitted); err != nil {
		http.Error(w, "Invalid dashboard layout: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		http.Error(w, "Invalid dashboard layout: multiple JSON values", http.StatusBadRequest)
		return
	}

	normalized, err := validateAdminDashboardLayout(submitted)
	if err != nil {
		http.Error(w, "Invalid dashboard layout: "+err.Error(), http.StatusBadRequest)
		return
	}

	adminDashboardLayoutMu.Lock()
	defer adminDashboardLayoutMu.Unlock()
	settings, err := h.configManager.Load()
	if err != nil {
		http.Error(w, "Failed to load settings", http.StatusInternalServerError)
		return
	}
	settings.UI.AdminDashboardLayout = &normalized
	if err := h.configManager.Save(settings); err != nil {
		http.Error(w, "Failed to save dashboard layout", http.StatusInternalServerError)
		return
	}
	writeDashboardLayoutJSON(w, http.StatusOK, normalized)
}

// ResetDashboardLayout removes the saved arrangement and returns the built-in default.
func (h *AdminUIHandler) ResetDashboardLayout(w http.ResponseWriter, _ *http.Request) {
	if h.configManager == nil {
		http.Error(w, "Configuration manager not available", http.StatusInternalServerError)
		return
	}

	adminDashboardLayoutMu.Lock()
	defer adminDashboardLayoutMu.Unlock()
	settings, err := h.configManager.Load()
	if err != nil {
		http.Error(w, "Failed to load settings", http.StatusInternalServerError)
		return
	}
	settings.UI.AdminDashboardLayout = nil
	if err := h.configManager.Save(settings); err != nil {
		http.Error(w, "Failed to reset dashboard layout", http.StatusInternalServerError)
		return
	}
	writeDashboardLayoutJSON(w, http.StatusOK, defaultAdminDashboardLayout())
}

func writeDashboardLayoutJSON(w http.ResponseWriter, status int, layout config.AdminDashboardLayout) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(dashboardLayoutResponse(layout))
}
