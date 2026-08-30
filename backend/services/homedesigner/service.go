package homedesigner

import (
	"context"
	"errors"
	"fmt"

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
	config    *config.Manager
	profiles  *user_settings.Service
	directory UserDirectory
}

func New(manager *config.Manager, profiles *user_settings.Service, directory UserDirectory) *Service {
	return &Service{config: manager, profiles: profiles, directory: directory}
}

// Load returns the editor document for the requested global or profile scope.
func (s *Service) Load(_ context.Context, actor Actor, scope Scope) (Document, error) {
	if err := s.authorize(actor, scope); err != nil {
		return Document{}, err
	}
	settings, err := s.config.Load()
	if err != nil {
		return Document{}, err
	}

	globalRows := configHomeShelvesToModel(settings.HomeShelves)
	globalTheme := ConfigAppearanceToModel(settings.Display.Appearance)
	if scope.Kind == "global" {
		return Document{
			Scope: scope, Revision: RevisionForGlobal(settings),
			Rows:    RowsSection{Effective: globalRows, Override: &globalRows},
			Theme:   ThemeSection{Effective: globalTheme, Override: &globalTheme},
			Catalog: BuildCatalog(settings, s.directory.ListAll()),
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
		Scope: scope, Revision: RevisionForProfile(raw),
		Rows:    RowsSection{Inherited: raw == nil || homeShelvesInherited(raw.HomeShelves), Effective: effective.HomeShelves},
		Theme:   ThemeSection{Inherited: raw == nil || appearanceInherited(raw.Display.Appearance), Effective: effective.Display.Appearance},
		Catalog: BuildCatalog(settings, s.directory.ListAll()),
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
	if fields := ValidateApply(request, BuildCatalog(settings, s.directory.ListAll())); len(fields) > 0 {
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
