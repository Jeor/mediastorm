package homedesigner

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"novastream/config"
	"novastream/models"
	"novastream/services/user_settings"
)

var (
	ErrForbidden        = errors.New("home designer access is forbidden")
	ErrProfileNotFound  = errors.New("home designer profile not found")
	ErrRevisionConflict = errors.New("home designer document changed before this update")
)

// ValidationError reports fields which cannot safely be applied to existing
// persisted settings.
type ValidationError struct {
	Fields []FieldError
}

func (e ValidationError) Error() string {
	return "home designer validation failed"
}

// UserDirectory supplies profile ownership facts to the Home Designer service.
// It deliberately exposes only the profile/account operations required here.
type UserDirectory interface {
	Get(string) (models.User, bool)
	ListAll() []models.User
	ListForAccount(string) []models.User
	BelongsToAccount(profileID, accountID string) bool
}

// Service coordinates authorized, scoped Home Designer reads and writes.
type Service struct {
	config             *config.Manager
	profiles           *user_settings.Service
	directory          UserDirectory
	capabilityProvider CatalogCapabilityProvider
}

func New(manager *config.Manager, profiles *user_settings.Service, directory UserDirectory) *Service {
	return NewWithCatalogCapabilities(manager, profiles, directory, nil)
}

// NewWithCatalogCapabilities constructs a service with the explicit safe
// capability resolver required for account-scoped catalog entries.
func NewWithCatalogCapabilities(manager *config.Manager, profiles *user_settings.Service, directory UserDirectory, provider CatalogCapabilityProvider) *Service {
	return &Service{config: manager, profiles: profiles, directory: directory, capabilityProvider: provider}
}

// Load returns the editor document for the requested global or profile scope.
func (s *Service) Load(ctx context.Context, actor Actor, scope Scope) (Document, error) {
	if err := s.authorize(actor, scope); err != nil {
		return Document{}, err
	}
	settings, err := s.config.Load()
	if err != nil {
		return Document{}, err
	}

	globalRows := configHomeShelvesToModel(settings.HomeShelves)
	globalTheme := ConfigAppearanceToModel(settings.Display.Appearance)
	envelope := s.documentEnvelope(ctx, actor, scope)
	if scope.Kind == "global" {
		return Document{
			Scope: scope, Permissions: envelope.permissions, PreviewProfiles: envelope.previewProfiles, Revision: RevisionForGlobal(settings),
			Rows:    RowsSection{Effective: globalRows, Override: &globalRows},
			Theme:   ThemeSection{Effective: globalTheme, Override: &globalTheme},
			Catalog: BuildCatalogForContext(settings, envelope.catalogContext), ThemePresets: themePresets(),
		}, nil
	}

	raw, err := s.profiles.Get(scope.ProfileID)
	if err != nil {
		return Document{}, err
	}
	defaults := models.DefaultUserSettings()
	defaults.HomeShelves = globalRows
	defaults.Display.Appearance = globalTheme
	effective, err := s.profiles.GetWithDefaults(scope.ProfileID, defaults)
	if err != nil {
		return Document{}, err
	}

	document := Document{
		Scope: scope, Permissions: envelope.permissions, PreviewProfiles: envelope.previewProfiles, Revision: RevisionForProfile(raw),
		Rows:    RowsSection{Inherited: raw == nil || homeShelvesInherited(raw.HomeShelves), Effective: effective.HomeShelves},
		Theme:   ThemeSection{Inherited: raw == nil || appearanceInherited(raw.Display.Appearance), Effective: effective.Display.Appearance},
		Catalog: BuildCatalogForContext(settings, envelope.catalogContext), ThemePresets: themePresets(),
	}
	if raw != nil {
		if !document.Rows.Inherited {
			override := raw.HomeShelves
			document.Rows.Override = &override
		}
		if !document.Theme.Inherited {
			override := raw.Display.Appearance
			document.Theme.Override = &override
		}
	}
	return document, nil
}

// Apply validates and atomically persists the requested scoped sections. The
// resulting document is reloaded so its effective values and revision exactly
// match a subsequent editor load.
func (s *Service) Apply(ctx context.Context, actor Actor, request ApplyRequest) (Document, error) {
	if err := s.authorize(actor, request.Scope); err != nil {
		return Document{}, err
	}
	settings, err := s.config.Load()
	if err != nil {
		return Document{}, err
	}
	if fields := ValidateApply(request, BuildCatalogForContext(settings, s.catalogContext(ctx, actor, request.Scope))); len(fields) > 0 {
		return Document{}, ValidationError{Fields: fields}
	}

	switch request.Scope.Kind {
	case "global":
		err = s.config.Mutate(func(current *config.Settings) error {
			if request.ExpectedRevision != RevisionForGlobal(*current) {
				return ErrRevisionConflict
			}
			if request.Rows != nil {
				current.HomeShelves = applyModelHomeShelves(current.HomeShelves, *request.Rows.Value)
			}
			if request.Theme != nil {
				current.Display.Appearance = ModelAppearanceToConfig(*request.Theme.Value)
			}
			return nil
		})
	case "profile":
		err = s.profiles.Mutate(request.Scope.ProfileID, func(current *models.UserSettings) error {
			if request.ExpectedRevision != RevisionForProfile(current) {
				return ErrRevisionConflict
			}
			if request.Rows != nil {
				if request.Rows.Mode == ModeInherit {
					current.HomeShelves = models.HomeShelvesSettings{}
				} else {
					current.HomeShelves = *request.Rows.Value
				}
			}
			if request.Theme != nil {
				if request.Theme.Mode == ModeInherit {
					current.Display.Appearance = models.AppearanceSettings{}
				} else {
					current.Display.Appearance = *request.Theme.Value
				}
			}
			return nil
		})
	default:
		// authorize already validates the scope kind.
		return Document{}, ValidationError{Fields: []FieldError{{Section: "scope", Path: "kind", Message: "scope kind must be global or profile"}}}
	}
	if err != nil {
		return Document{}, err
	}
	return s.Load(ctx, actor, request.Scope)
}

type documentEnvelope struct {
	permissions     ScopePermissions
	previewProfiles []PreviewProfile
	catalogContext  CatalogContext
}

func (s *Service) documentEnvelope(ctx context.Context, actor Actor, scope Scope) documentEnvelope {
	return documentEnvelope{
		permissions:     ScopePermissions{CanEdit: true, CanEditGlobal: actor.IsAdmin, CanEditProfiles: true},
		previewProfiles: s.previewProfiles(actor),
		catalogContext:  s.catalogContext(ctx, actor, scope),
	}
}

func (s *Service) previewProfiles(actor Actor) []PreviewProfile {
	users := s.directory.ListForAccount(actor.AccountID)
	if actor.IsAdmin {
		users = s.directory.ListAll()
	}
	profiles := make([]PreviewProfile, 0, len(users))
	for _, user := range users {
		id := strings.TrimSpace(user.ID)
		if id == "" {
			continue
		}
		profiles = append(profiles, PreviewProfile{ID: id, DisplayName: strings.TrimSpace(user.Name)})
	}
	return profiles
}

func (s *Service) catalogContext(ctx context.Context, actor Actor, scope Scope) CatalogContext {
	context := CatalogContext{Actor: actor}
	if s.capabilityProvider == nil {
		return context
	}
	capabilities := s.capabilityProvider.CatalogCapabilities(ctx, actor, scope)
	context.Libraries = append([]CatalogLibrary(nil), capabilities.Libraries...)
	context.AuthorizedAccounts = append([]CatalogAccountAuthorization(nil), capabilities.AuthorizedAccounts...)
	context.BasePath = capabilities.BasePath
	return context
}

func (s *Service) authorize(actor Actor, scope Scope) error {
	switch scope.Kind {
	case "global":
		if !actor.IsAdmin {
			return ErrForbidden
		}
		return nil
	case "profile":
		if scope.ProfileID == "" {
			return ValidationError{Fields: []FieldError{{Section: "scope", Path: "profileId", Message: "profile scope requires a profile id"}}}
		}
		if _, found := s.directory.Get(scope.ProfileID); !found {
			return ErrProfileNotFound
		}
		if actor.IsAdmin || s.directory.BelongsToAccount(scope.ProfileID, actor.AccountID) {
			return nil
		}
		return ErrProfileNotFound
	default:
		return ValidationError{Fields: []FieldError{{Section: "scope", Path: "kind", Message: fmt.Sprintf("unsupported scope kind %q", scope.Kind)}}}
	}
}

func homeShelvesInherited(settings models.HomeShelvesSettings) bool {
	return len(settings.Shelves) == 0 && settings.ExploreCardPosition == "" && settings.ItemCap == 0 &&
		settings.ExcludeUpcomingFromContinue == nil && settings.MobileTopShelfMode == "" && settings.MobileTopShelfSourceID == "" &&
		settings.TVTopShelfMode == "" && settings.TVTopShelfSourceID == "" && settings.DisableTvLandscapeCardExpansion == nil &&
		settings.HomeShelfScale == nil && settings.HomeHeroScale == nil
}

func appearanceInherited(settings models.AppearanceSettings) bool {
	return settings.FontScale == nil && settings.AccentColor == "" && settings.TextColor == "" &&
		settings.SecondaryTextColor == "" && settings.BackgroundColor == "" && settings.ModalBackgroundColor == "" &&
		settings.ButtonStyle == "" && settings.ButtonRadius == "" && settings.HighContrast == nil && settings.ReduceOverlays == nil
}

func configHomeShelvesToModel(settings config.HomeShelvesSettings) models.HomeShelvesSettings {
	return models.HomeShelvesSettings{
		Shelves:                         ConfigShelvesToModels(settings.Shelves),
		ExploreCardPosition:             string(settings.ExploreCardPosition),
		ItemCap:                         settings.ItemCap,
		ExcludeUpcomingFromContinue:     models.BoolPtr(settings.ExcludeUpcomingFromContinue),
		MobileTopShelfMode:              settings.MobileTopShelfMode,
		MobileTopShelfSourceID:          settings.MobileTopShelfSourceID,
		TVTopShelfMode:                  settings.TVTopShelfMode,
		TVTopShelfSourceID:              settings.TVTopShelfSourceID,
		DisableTvLandscapeCardExpansion: models.BoolPtr(settings.DisableTvLandscapeCardExpansion),
		HomeShelfScale:                  models.FloatPtr(settings.HomeShelfScale),
		HomeHeroScale:                   models.FloatPtr(settings.HomeHeroScale),
	}
}

func applyModelHomeShelves(current config.HomeShelvesSettings, settings models.HomeShelvesSettings) config.HomeShelvesSettings {
	current.Shelves = ModelShelvesToConfig(settings.Shelves)
	current.ExploreCardPosition = config.ExploreCardPosition(settings.ExploreCardPosition)
	current.ItemCap = settings.ItemCap
	current.ExcludeUpcomingFromContinue = models.BoolVal(settings.ExcludeUpcomingFromContinue, false)
	current.MobileTopShelfMode = settings.MobileTopShelfMode
	current.MobileTopShelfSourceID = settings.MobileTopShelfSourceID
	current.TVTopShelfMode = settings.TVTopShelfMode
	current.TVTopShelfSourceID = settings.TVTopShelfSourceID
	current.DisableTvLandscapeCardExpansion = models.BoolVal(settings.DisableTvLandscapeCardExpansion, false)
	current.HomeShelfScale = models.FloatVal(settings.HomeShelfScale, 0)
	current.HomeHeroScale = models.FloatVal(settings.HomeHeroScale, 0)
	return current
}

func themePresets() []ThemePreset {
	return []ThemePreset{
		newThemePreset("default-dark", "Default Dark", "Standard dark theme with blue accent and default sizing.", 1, "#3f66ff", "#ffffff", "#c7cad6", "#0b0b0f", "#1f1f2a", "soft", "rounded", false, false),
		newThemePreset("amoled-black", "AMOLED Black", "Pure black page and modal backgrounds for OLED displays.", 1, "#3f66ff", "#ffffff", "#b8bdcf", "#000000", "#000000", "soft", "rounded", false, true),
		newThemePreset("storm-gold", "Storm Gold", "Yellow lightning accent derived from the new thunderbolt branding.", 1, "#f0d020", "#f7f7f2", "#b8bdc7", "#090b0e", "#1b2028", "soft", "rounded", false, false),
		newThemePreset("high-contrast", "High Contrast", "Brighter text, stronger borders, filled buttons, and flatter overlays.", 1.15, "#ffd400", "#ffffff", "#e6e8f2", "#0b0b0f", "#101018", "filled", "rounded", true, true),
		newThemePreset("large-accessible", "Large Accessible", "Larger text with calmer contrast for readability testing.", 1.3, "#22c55e", "#f8fafc", "#cbd5e1", "#0f172a", "#172033", "soft", "pill", false, true),
		newThemePreset("ocean-cyan", "Ocean Cyan", "Cool blue modal surfaces with cyan accents and soft buttons.", 1, "#00d4ff", "#e6fbff", "#8bd8e8", "#031521", "#082f49", "soft", "rounded", false, false),
		newThemePreset("rose-pop", "Rose Pop", "Pink accent theme with deep berry modal surfaces.", 1, "#ff4fa3", "#fff1f7", "#f9a8d4", "#1f0714", "#4a1230", "filled", "pill", false, false),
		newThemePreset("ember-gold", "Ember Gold", "Warm gold accent with dark red modal surfaces.", 1, "#ffb000", "#fff7ed", "#fdba74", "#1c0703", "#431407", "outlined", "rounded", false, true),
		newThemePreset("terminal-lime", "Terminal Lime", "Green-on-dark styling for obvious text and accent checks.", 1, "#7cff00", "#d9ffb3", "#8ee66b", "#000000", "#061a0b", "outlined", "square", true, true),
		newThemePreset("scan-bright", "Bright Scan", "Deliberately loud colors for spotting screens that ignore theme values.", 1, "#00ff66", "#ff1a1a", "#00ffff", "#120021", "#2b0052", "outlined", "square", false, false),
	}
}

func newThemePreset(id, name, description string, fontScale float64, accentColor, textColor, secondaryTextColor, backgroundColor, modalBackgroundColor, buttonStyle, buttonRadius string, highContrast, reduceOverlays bool) ThemePreset {
	return ThemePreset{
		ID: id, Name: name, Description: description,
		Appearance: models.AppearanceSettings{
			FontScale:            models.FloatPtr(fontScale),
			AccentColor:          accentColor,
			TextColor:            textColor,
			SecondaryTextColor:   secondaryTextColor,
			BackgroundColor:      backgroundColor,
			ModalBackgroundColor: modalBackgroundColor,
			ButtonStyle:          buttonStyle,
			ButtonRadius:         buttonRadius,
			HighContrast:         models.BoolPtr(highContrast),
			ReduceOverlays:       models.BoolPtr(reduceOverlays),
		},
	}
}
