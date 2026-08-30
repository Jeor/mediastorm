package homedesigner

import "novastream/models"

const (
	ModeCustom  = "custom"
	ModeInherit = "inherit"
)

// Scope identifies the persisted Home Designer configuration being edited.
type Scope struct {
	Kind      string `json:"kind"`
	ProfileID string `json:"profileId,omitempty"`
}

// Actor captures the caller facts used by the service authorization layer.
type Actor struct {
	IsAdmin   bool   `json:"isAdmin"`
	AccountID string `json:"accountId"`
}

type RowsSection struct {
	Inherited bool                        `json:"inherited"`
	Effective models.HomeShelvesSettings  `json:"effective"`
	Override  *models.HomeShelvesSettings `json:"override,omitempty"`
}

type ThemeSection struct {
	Inherited bool                       `json:"inherited"`
	Effective models.AppearanceSettings  `json:"effective"`
	Override  *models.AppearanceSettings `json:"override,omitempty"`
}

// Document is the stable Home Designer response returned to editor clients.
type Document struct {
	Scope    Scope          `json:"scope"`
	Revision string         `json:"revision"`
	Rows     RowsSection    `json:"rows"`
	Theme    ThemeSection   `json:"theme"`
	Catalog  []CatalogEntry `json:"catalog"`
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type CatalogField struct {
	Path     string   `json:"path"`
	Type     string   `json:"type"`
	Label    string   `json:"label"`
	Required bool     `json:"required"`
	Options  []Option `json:"options,omitempty"`
}

type CatalogEntry struct {
	Type              string             `json:"type"`
	Name              string             `json:"name"`
	Description       string             `json:"description"`
	Category          string             `json:"category"`
	Multiple          bool               `json:"multiple"`
	Available         bool               `json:"available"`
	UnavailableReason string             `json:"unavailableReason,omitempty"`
	SetupPath         string             `json:"setupPath,omitempty"`
	Default           models.ShelfConfig `json:"default"`
	Fields            []CatalogField     `json:"fields"`
	PreviewKind       string             `json:"previewKind"`
}

type SectionMutation[T any] struct {
	Mode  string `json:"mode"`
	Value *T     `json:"value,omitempty"`
}

type ApplyRequest struct {
	Scope            Scope                                        `json:"scope"`
	ExpectedRevision string                                       `json:"expectedRevision"`
	Rows             *SectionMutation[models.HomeShelvesSettings] `json:"rows,omitempty"`
	Theme            *SectionMutation[models.AppearanceSettings]  `json:"theme,omitempty"`
}

type PreviewRequest struct {
	Scope Scope                                        `json:"scope"`
	Rows  *SectionMutation[models.HomeShelvesSettings] `json:"rows,omitempty"`
	Theme *SectionMutation[models.AppearanceSettings]  `json:"theme,omitempty"`
}

type PreviewResponse struct {
	Rows  models.HomeShelvesSettings `json:"rows"`
	Theme models.AppearanceSettings  `json:"theme"`
}

type FieldError struct {
	Section string `json:"section"`
	RowID   string `json:"rowId,omitempty"`
	Path    string `json:"path"`
	Message string `json:"message"`
}
