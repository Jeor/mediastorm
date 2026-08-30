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

// CatalogLibrary is the presentation-safe representation of a library the
// caller is authorized to configure. Callers must not pass inaccessible
// libraries or filesystem paths into this contract.
type CatalogLibrary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CatalogContext carries the authorization and navigation context required to
// construct a profile-safe catalog. Shared integrations are opt-in because an
// empty owner is not by itself permission to expose an account.
type CatalogContext struct {
	Actor                 Actor            `json:"-"`
	Profiles              []models.User    `json:"-"`
	Libraries             []CatalogLibrary `json:"-"`
	BasePath              string           `json:"-"`
	IncludeSharedAccounts bool             `json:"-"`
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
	Scope            Scope                                        `json:"scope"`
	PreviewProfileID string                                       `json:"previewProfileId"`
	Platform         string                                       `json:"platform"`
	Rows             *SectionMutation[models.HomeShelvesSettings] `json:"rows,omitempty"`
	Theme            *SectionMutation[models.AppearanceSettings]  `json:"theme,omitempty"`
}

// PreviewRow intentionally includes only data a client may render. It never
// mirrors source URLs, integration IDs, account IDs, or transport settings.
type PreviewRow struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	Order          int    `json:"order"`
	Type           string `json:"type"`
	Limit          int    `json:"limit,omitempty"`
	HideUnreleased bool   `json:"hideUnreleased,omitempty"`
}

type PreviewTheme struct {
	FontScale            *float64 `json:"fontScale,omitempty"`
	AccentColor          string   `json:"accentColor,omitempty"`
	TextColor            string   `json:"textColor,omitempty"`
	SecondaryTextColor   string   `json:"secondaryTextColor,omitempty"`
	BackgroundColor      string   `json:"backgroundColor,omitempty"`
	ModalBackgroundColor string   `json:"modalBackgroundColor,omitempty"`
	ButtonStyle          string   `json:"buttonStyle,omitempty"`
	ButtonRadius         string   `json:"buttonRadius,omitempty"`
	HighContrast         *bool    `json:"highContrast,omitempty"`
	ReduceOverlays       *bool    `json:"reduceOverlays,omitempty"`
}

type PreviewResponse struct {
	Scope     Scope        `json:"scope"`
	ProfileID string       `json:"profileId"`
	Platform  string       `json:"platform"`
	Rows      []PreviewRow `json:"rows"`
	Theme     PreviewTheme `json:"theme"`
}

// BuildPreviewResponse projects persisted settings into the deliberately
// narrow preview response contract.
func BuildPreviewResponse(request PreviewRequest, rows models.HomeShelvesSettings, theme models.AppearanceSettings) PreviewResponse {
	previewRows := make([]PreviewRow, len(rows.Shelves))
	for i, row := range rows.Shelves {
		previewRows[i] = PreviewRow{ID: row.ID, Name: row.Name, Enabled: row.Enabled, Order: row.Order, Type: row.Type, Limit: row.Limit, HideUnreleased: row.HideUnreleased}
	}
	return PreviewResponse{
		Scope: request.Scope, ProfileID: request.PreviewProfileID, Platform: request.Platform, Rows: previewRows,
		Theme: PreviewTheme{FontScale: theme.FontScale, AccentColor: theme.AccentColor, TextColor: theme.TextColor, SecondaryTextColor: theme.SecondaryTextColor, BackgroundColor: theme.BackgroundColor, ModalBackgroundColor: theme.ModalBackgroundColor, ButtonStyle: theme.ButtonStyle, ButtonRadius: theme.ButtonRadius, HighContrast: theme.HighContrast, ReduceOverlays: theme.ReduceOverlays},
	}
}

type FieldError struct {
	Section string `json:"section"`
	RowID   string `json:"rowId,omitempty"`
	Path    string `json:"path"`
	Message string `json:"message"`
}
