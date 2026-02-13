package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
	"github.com/technobecet/kaizoku-go/internal/config"
	"github.com/technobecet/kaizoku-go/internal/ent"
	"github.com/technobecet/kaizoku-go/internal/ent/providerstorage"
	"github.com/technobecet/kaizoku-go/internal/service/suwayomi"
	"github.com/technobecet/kaizoku-go/internal/types"
)

type ProviderHandler struct {
	config   *config.Config
	db       *ent.Client
	suwayomi *suwayomi.Client

	mu    sync.RWMutex
	cache []suwayomi.SuwayomiExtension
}

func (h *ProviderHandler) GetProviders(c echo.Context) error {
	ctx := c.Request().Context()

	// Sync all provider storage entries (like .NET's GetCachedProvidersAsync)
	// This ensures ProviderStorage DB entries are up-to-date before returning.
	if err := h.syncAllProviderStorage(ctx); err != nil {
		log.Warn().Err(err).Msg("failed to sync provider storage on list")
	}

	extensions, err := h.suwayomi.GetExtensions(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Deduplicate: keep latest version per extension name
	grouped := make(map[string]suwayomi.SuwayomiExtension)
	for _, ext := range extensions {
		existing, ok := grouped[ext.Name]
		if !ok || ext.VersionCode > existing.VersionCode {
			grouped[ext.Name] = ext
		}
	}

	result := make([]suwayomi.SuwayomiExtension, 0, len(grouped))
	for _, ext := range grouped {
		// Rewrite icon URL to our proxy
		ext.IconURL = "/api/provider/icon/" + ext.ApkName
		result = append(result, ext)
	}

	// Sort by name, then lang ("all" first)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		li := result[i].Lang
		lj := result[j].Lang
		if li == "all" {
			li = "!"
		}
		if lj == "all" {
			lj = "!"
		}
		return li < lj
	})

	h.mu.Lock()
	h.cache = result
	h.mu.Unlock()

	return c.JSON(http.StatusOK, result)
}

func (h *ProviderHandler) InstallProvider(c echo.Context) error {
	pkgName := c.Param("pkg")
	if pkgName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "package name required"})
	}

	ctx := c.Request().Context()
	if err := h.suwayomi.InstallExtension(ctx, pkgName); err != nil {
		log.Error().Err(err).Str("pkg", pkgName).Msg("failed to install extension")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Create/update ProviderStorage entry for the installed extension
	if err := h.syncProviderStorage(ctx, pkgName); err != nil {
		log.Warn().Err(err).Str("pkg", pkgName).Msg("failed to sync provider storage after install")
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Extension installed successfully"})
}

func (h *ProviderHandler) InstallProviderFile(c echo.Context) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "file required"})
	}

	src, err := file.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to open file"})
	}
	defer src.Close()

	ctx := c.Request().Context()
	if err := h.suwayomi.InstallExtensionFromFile(ctx, file.Filename, src); err != nil {
		log.Error().Err(err).Str("filename", file.Filename).Msg("failed to install extension from file")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Sync provider storage for all extensions (we don't know the pkgName from file install)
	if err := h.syncAllProviderStorage(ctx); err != nil {
		log.Warn().Err(err).Msg("failed to sync provider storage after file install")
	}

	// Return the APK name (filename without path)
	apkName := file.Filename
	return c.String(http.StatusOK, apkName)
}

func (h *ProviderHandler) UninstallProvider(c echo.Context) error {
	pkgName := c.Param("pkg")
	if pkgName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "package name required"})
	}

	ctx := c.Request().Context()
	if err := h.suwayomi.UninstallExtension(ctx, pkgName); err != nil {
		log.Error().Err(err).Str("pkg", pkgName).Msg("failed to uninstall extension")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// Mark the ProviderStorage entry as disabled
	providers, err := h.db.ProviderStorage.Query().All(ctx)
	if err == nil {
		for _, p := range providers {
			if p.PkgName == pkgName || p.ApkName == pkgName {
				_, _ = h.db.ProviderStorage.UpdateOneID(p.ID).
					SetIsDisabled(true).
					Save(ctx)
				break
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Extension uninstalled successfully"})
}

func (h *ProviderHandler) GetProviderPreferences(c echo.Context) error {
	pkgName := c.Param("pkg")
	if pkgName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "package name required"})
	}

	ctx := c.Request().Context()

	// Find provider in DB
	providers, err := h.db.ProviderStorage.Query().All(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	var matchedProvider *ent.ProviderStorage
	for _, p := range providers {
		if p.PkgName == pkgName || p.ApkName == pkgName {
			matchedProvider = p
			break
		}
	}

	if matchedProvider == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "provider not found"})
	}

	type ProviderPreference struct {
		Type         int         `json:"type"`
		Key          string      `json:"key"`
		Title        string      `json:"title"`
		Summary      *string     `json:"summary"`
		ValueType    int         `json:"valueType"`
		DefaultValue interface{} `json:"defaultValue"`
		Entries      []string    `json:"entries"`
		EntryValues  []string    `json:"entryValues"`
		CurrentValue interface{} `json:"currentValue"`
		Source       *string     `json:"source"`
	}

	prefs := make([]ProviderPreference, 0)

	// Storage preference (built-in) — matches .NET key "isStorage"
	storageType := "permanent"
	if !matchedProvider.IsStorage {
		storageType = "temporary"
	}
	summary := "Permanent providers always download new chapters and replace any existing copies from temporary providers.\nTemporary providers only download a chapter if they are the first to have it available."
	prefs = append(prefs, ProviderPreference{
		Type:         entryTypeComboBox,
		Key:          "isStorage",
		Title:        "Provider Download Defaults",
		Summary:      &summary,
		ValueType:    valueTypeString,
		DefaultValue: "permanent",
		Entries:      []string{"Permanent", "Temporary"},
		EntryValues:  []string{"permanent", "temporary"},
		CurrentValue: storageType,
	})

	// Collect unique preferences ordered English first, then fetch fresh values from Suwayomi
	seen := make(map[string]bool)
	type prefEntry struct {
		key      string
		sourceID string
	}
	var orderedPrefs []prefEntry

	// Order mappings: English first
	mappings := orderMappingsEnglishFirst(matchedProvider.Mappings)
	for _, m := range mappings {
		if m.Source == nil {
			continue
		}
		for _, sp := range m.Preferences {
			if !seen[sp.Props.Key] {
				seen[sp.Props.Key] = true
				orderedPrefs = append(orderedPrefs, prefEntry{key: sp.Props.Key, sourceID: m.Source.ID})
			}
		}
	}

	// Fetch fresh preferences from Suwayomi for each source
	freshPrefs := make(map[string][]suwayomi.SuwayomiPreference)
	sourcesSeen := make(map[string]bool)
	for _, pe := range orderedPrefs {
		if sourcesSeen[pe.sourceID] {
			continue
		}
		sourcesSeen[pe.sourceID] = true
		sp, err := h.suwayomi.GetSourcePreferences(ctx, pe.sourceID)
		if err != nil {
			log.Warn().Err(err).Str("source", pe.sourceID).Msg("failed to fetch fresh preferences")
			continue
		}
		// Remove suffix for "all" lang extensions
		if matchedProvider.Lang == "all" {
			for i := range sp {
				if idx := strings.LastIndex(sp[i].Props.Key, "_"); idx > 0 {
					sp[i].Props.Key = sp[i].Props.Key[:idx]
				}
			}
		}
		freshPrefs[pe.sourceID] = sp
	}

	// Build preferences using fresh values
	for _, pe := range orderedPrefs {
		fresh, ok := freshPrefs[pe.sourceID]
		if !ok {
			continue
		}
		for _, fp := range fresh {
			if fp.Props.Key == pe.key {
				entryType, valType := mapPreferenceTypeInt(fp.Type)
				sourceID := pe.sourceID
				pref := ProviderPreference{
					Type:         entryType,
					Key:          fp.Props.Key,
					Title:        fp.Props.Title,
					ValueType:    valType,
					DefaultValue: fp.Props.DefaultValue,
					Entries:      fp.Props.Entries,
					EntryValues:  fp.Props.EntryValues,
					CurrentValue: fp.Props.CurrentValue,
					Source:       &sourceID,
				}
				if fp.Props.Summary != "" {
					pref.Summary = &fp.Props.Summary
				}
				// Handle empty entry values
				if len(pref.Entries) > 0 {
					pref.EntryValues = replaceEmptyEntryValues(pref.EntryValues)
					if str, ok := pref.CurrentValue.(string); ok && str == "" {
						pref.CurrentValue = "!empty-value!"
					}
					if str, ok := pref.DefaultValue.(string); ok && str == "" {
						pref.DefaultValue = "!empty-value!"
					}
					if pref.DefaultValue == nil && len(pref.EntryValues) > 0 {
						pref.DefaultValue = pref.EntryValues[0]
					}
					if pref.CurrentValue == nil {
						pref.CurrentValue = pref.DefaultValue
					}
				}
				if pref.Title == "" {
					pref.Title = fp.Props.DialogTitle
				}
				prefs = append(prefs, pref)
				break
			}
		}
	}

	resp := struct {
		ApkName     string               `json:"apkName"`
		Preferences []ProviderPreference `json:"preferences"`
	}{
		ApkName:     matchedProvider.ApkName,
		Preferences: prefs,
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *ProviderHandler) SetProviderPreferences(c echo.Context) error {
	var req struct {
		ApkName     string `json:"apkName"`
		Preferences []struct {
			Type         int         `json:"type"`
			Key          string      `json:"key"`
			ValueType    int         `json:"valueType"`
			CurrentValue interface{} `json:"currentValue"`
			Source       *string     `json:"source"`
		} `json:"preferences"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	ctx := c.Request().Context()

	// Find provider in DB
	providers, err := h.db.ProviderStorage.Query().All(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	var provider *ent.ProviderStorage
	for _, p := range providers {
		if p.ApkName == req.ApkName {
			provider = p
			break
		}
	}
	if provider == nil {
		return c.JSON(http.StatusOK, map[string]string{"message": "Provider not found"})
	}

	// Collect non-storage preferences that need updating
	type updateEntry struct {
		key   string
		value interface{}
	}
	var toUpdate []updateEntry

	for _, pref := range req.Preferences {
		// Handle storage preference
		if pref.Key == "isStorage" {
			storageValue := convertJSONValue(pref.CurrentValue)
			isStorage := false
			if sv, ok := storageValue.(string); ok {
				isStorage = sv == "permanent"
			}
			if provider.IsStorage != isStorage {
				_, _ = h.db.ProviderStorage.UpdateOneID(provider.ID).
					SetIsStorage(isStorage).
					Save(ctx)
			}
			continue
		}

		if pref.Source == nil || pref.CurrentValue == nil {
			continue
		}

		// Fetch current preferences from Suwayomi for this source (to detect changes)
		sourcePrefs, err := h.suwayomi.GetSourcePreferences(ctx, *pref.Source)
		if err != nil {
			log.Warn().Err(err).Str("source", *pref.Source).Msg("failed to get source preferences")
			continue
		}
		// Remove suffix for "all" lang extensions
		if provider.Lang == "all" {
			for i := range sourcePrefs {
				if idx := strings.LastIndex(sourcePrefs[i].Props.Key, "_"); idx > 0 {
					sourcePrefs[i].Props.Key = sourcePrefs[i].Props.Key[:idx]
				}
			}
		}

		// Find the matching preference and compare values
		for _, sp := range sourcePrefs {
			if sp.Props.Key != pref.Key {
				continue
			}

			newVal := convertJSONValue(pref.CurrentValue)
			curVal := convertJSONValue(sp.Props.CurrentValue)

			// Convert "!empty-value!" sentinel back to "" for ComboBox
			if pref.ValueType == valueTypeString && pref.Type == entryTypeComboBox {
				if sv, ok := newVal.(string); ok && sv == "!empty-value!" {
					newVal = ""
				}
			}

			changed := false
			switch pref.ValueType {
			case valueTypeString:
				ns, _ := newVal.(string)
				cs, _ := curVal.(string)
				changed = ns != cs
			case valueTypeBoolean:
				nb := toBool(newVal)
				cb := toBool(curVal)
				changed = nb != cb
			case valueTypeStringCollection:
				na := toStringSlice(newVal)
				ca := toStringSlice(curVal)
				changed = !stringSliceEqual(na, ca)
			default:
				changed = true
			}

			if changed {
				toUpdate = append(toUpdate, updateEntry{key: pref.Key, value: newVal})
			}
			break
		}
	}

	// Apply updates to ALL sources of this provider (not just the one from the request)
	if len(toUpdate) > 0 {
		for _, mapping := range provider.Mappings {
			if mapping.Source == nil {
				continue
			}
			for _, upd := range toUpdate {
				// Find the preference index in this mapping's preferences
				for i, mp := range mapping.Preferences {
					if mp.Props.Key == upd.key {
						if err := h.suwayomi.SetSourcePreference(ctx, mapping.Source.ID, i, upd.value); err != nil {
							log.Warn().Err(err).
								Str("key", upd.key).
								Str("source", mapping.Source.ID).
								Msg("failed to set preference on source")
						}
						break
					}
				}
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Preferences set successfully"})
}

func (h *ProviderHandler) GetProviderIcon(c echo.Context) error {
	apkName := c.Param("apk")
	if apkName == "" {
		return c.NoContent(http.StatusNotFound)
	}

	// Strip any cache-busting suffix
	realApk := apkName
	if idx := strings.Index(apkName, "!"); idx >= 0 {
		realApk = apkName[:idx]
	}

	ctx := c.Request().Context()
	data, contentType, err := h.suwayomi.GetExtensionIcon(ctx, realApk)
	if err != nil {
		log.Warn().Err(err).Str("apk", realApk).Msg("failed to get extension icon")
		// Return a transparent 1x1 PNG as fallback
		return c.NoContent(http.StatusNotFound)
	}

	if contentType == "" {
		contentType = "image/png"
	}
	return c.Blob(http.StatusOK, contentType, data)
}

// Frontend enum values matching .NET's integer-serialized enums.
const (
	entryTypeComboBox      = 0
	entryTypeComboCheckBox = 1
	entryTypeTextBox       = 2
	entryTypeSwitch        = 3

	valueTypeString           = 0
	valueTypeStringCollection = 1
	valueTypeBoolean          = 2
)

// mapPreferenceTypeInt converts Suwayomi preference type to frontend integer enums.
func mapPreferenceTypeInt(suwayomiType string) (entryType, valueType int) {
	switch suwayomiType {
	case "ListPreference":
		return entryTypeComboBox, valueTypeString
	case "MultiSelectListPreference":
		return entryTypeComboCheckBox, valueTypeStringCollection
	case "SwitchPreferenceCompat", "TwoStatePreference", "CheckBoxPreference":
		return entryTypeSwitch, valueTypeBoolean
	default:
		return entryTypeTextBox, valueTypeString
	}
}

// orderMappingsEnglishFirst puts English-language mappings first.
func orderMappingsEnglishFirst(mappings []types.ProviderMapping) []types.ProviderMapping {
	result := make([]types.ProviderMapping, 0, len(mappings))
	var rest []types.ProviderMapping
	for _, m := range mappings {
		if m.Source != nil && m.Source.Lang == "en" {
			result = append(result, m)
		} else {
			rest = append(rest, m)
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		li, lj := "", ""
		if rest[i].Source != nil {
			li = rest[i].Source.Lang
		}
		if rest[j].Source != nil {
			lj = rest[j].Source.Lang
		}
		return li < lj
	})
	return append(result, rest...)
}

// replaceEmptyEntryValues replaces empty strings with "!empty-value!" sentinel.
func replaceEmptyEntryValues(vals []string) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		if v == "" {
			out[i] = "!empty-value!"
		} else {
			out[i] = v
		}
	}
	return out
}

// syncProviderStorage creates/updates ProviderStorage for a specific extension by pkgName.
func (h *ProviderHandler) syncProviderStorage(ctx context.Context, pkgName string) error {
	extensions, err := h.suwayomi.GetExtensions(ctx)
	if err != nil {
		return fmt.Errorf("get extensions: %w", err)
	}

	var ext *suwayomi.SuwayomiExtension
	for _, e := range extensions {
		if e.PkgName == pkgName {
			ext = &e
			break
		}
	}
	if ext == nil {
		return fmt.Errorf("extension %s not found in Suwayomi", pkgName)
	}

	sources, err := h.suwayomi.GetSources(ctx)
	if err != nil {
		return fmt.Errorf("get sources: %w", err)
	}

	return h.upsertProviderStorage(ctx, *ext, sources)
}

// syncAllProviderStorage creates/updates ProviderStorage for all installed extensions.
func (h *ProviderHandler) syncAllProviderStorage(ctx context.Context) error {
	extensions, err := h.suwayomi.GetExtensions(ctx)
	if err != nil {
		return fmt.Errorf("get extensions: %w", err)
	}

	sources, err := h.suwayomi.GetSources(ctx)
	if err != nil {
		return fmt.Errorf("get sources: %w", err)
	}

	for _, ext := range extensions {
		if ext.Installed {
			if err := h.upsertProviderStorage(ctx, ext, sources); err != nil {
				log.Warn().Err(err).Str("ext", ext.Name).Msg("failed to sync provider storage")
			}
		}
	}
	return nil
}

// upsertProviderStorage creates or updates a ProviderStorage entry for an extension.
func (h *ProviderHandler) upsertProviderStorage(ctx context.Context, ext suwayomi.SuwayomiExtension, sources []suwayomi.SuwayomiSource) error {
	// Match sources to this extension
	var matchedSources []suwayomi.SuwayomiSource
	for _, s := range sources {
		if ext.Lang == "all" {
			if s.Name == ext.Name {
				matchedSources = append(matchedSources, s)
			}
		} else {
			if s.Name == ext.Name && s.Lang == ext.Lang {
				matchedSources = append(matchedSources, s)
			}
		}
	}

	// Build mappings with preferences from Suwayomi
	mappings := make([]types.ProviderMapping, 0, len(matchedSources))
	for _, s := range matchedSources {
		prefs, err := h.suwayomi.GetSourcePreferences(ctx, s.ID)
		if err != nil {
			log.Warn().Err(err).Str("source", s.ID).Msg("failed to get source preferences")
			continue
		}

		typePrefs := make([]types.SuwayomiPreference, 0, len(prefs))
		for _, p := range prefs {
			key := p.Props.Key
			// Remove lang suffix for "all" lang extensions (like .NET does)
			if ext.Lang == "all" {
				if idx := strings.LastIndex(key, "_"); idx > 0 {
					key = key[:idx]
				}
			}
			typePrefs = append(typePrefs, types.SuwayomiPreference{
				Type: p.Type,
				Props: types.SuwayomiProp{
					Key:              key,
					Title:            p.Props.Title,
					Summary:          p.Props.Summary,
					DefaultValue:     p.Props.DefaultValue,
					Entries:          p.Props.Entries,
					EntryValues:      p.Props.EntryValues,
					DefaultValueType: p.Props.DefaultValueType,
					CurrentValue:     p.Props.CurrentValue,
					Visible:          p.Props.Visible,
					DialogTitle:      p.Props.DialogTitle,
					DialogMessage:    p.Props.DialogMessage,
					Text:             p.Props.Text,
				},
				Source: s.ID,
			})
		}

		src := &types.SuwayomiSource{
			ID:             s.ID,
			Name:           s.Name,
			Lang:           s.Lang,
			IconURL:        s.IconURL,
			SupportsLatest: s.SupportsLatest,
			IsConfigurable: s.IsConfigurable,
			IsNsfw:         s.IsNsfw,
			DisplayName:    s.DisplayName,
		}
		mappings = append(mappings, types.ProviderMapping{
			Source:      src,
			Preferences: typePrefs,
		})
	}

	// Check if provider already exists by name+lang
	existing, err := h.db.ProviderStorage.Query().
		Where(
			providerstorage.Name(ext.Name),
			providerstorage.Lang(ext.Lang),
		).
		Only(ctx)

	if err != nil {
		// Not found — create new entry
		_, err = h.db.ProviderStorage.Create().
			SetApkName(ext.ApkName).
			SetPkgName(ext.PkgName).
			SetName(ext.Name).
			SetLang(ext.Lang).
			SetVersionCode(ext.VersionCode).
			SetIsStorage(true).
			SetIsDisabled(false).
			SetMappings(mappings).
			Save(ctx)
		return err
	}

	// Update existing — preserve IsStorage setting
	_, err = h.db.ProviderStorage.UpdateOneID(existing.ID).
		SetApkName(ext.ApkName).
		SetPkgName(ext.PkgName).
		SetVersionCode(ext.VersionCode).
		SetIsDisabled(false).
		SetMappings(mappings).
		Save(ctx)
	return err
}

// convertJSONValue normalizes Go's JSON-decoded interface{} types.
// Go's encoding/json decodes numbers as float64, booleans as bool, arrays as []interface{}.
func convertJSONValue(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		return val
	case float64:
		// Could be a bool encoded as number
		if val == 0 {
			return false
		}
		if val == 1 {
			return true
		}
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return v
	}
}

// toBool converts an interface{} to bool.
func toBool(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case string:
		return val == "true"
	default:
		return false
	}
}

// toStringSlice converts an interface{} to []string.
func toStringSlice(v interface{}) []string {
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// stringSliceEqual compares two string slices.
func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// unknownIcon returns a minimal 1x1 transparent PNG.
func unknownIcon() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x62, 0x00, 0x00, 0x00, 0x02,
		0x00, 0x01, 0xe5, 0x27, 0xde, 0xfc, 0x00, 0x00,
		0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42,
		0x60, 0x82,
	}
}
