package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	RunnerProfileScopeAccount            = "account"
	RunnerProfileScopeGitHubInstallation = "github_installation"
)

func ValidateRunnerProfileScope(scope RunnerProfileScope) error {
	if scope.Type != RunnerProfileScopeAccount && scope.Type != RunnerProfileScopeGitHubInstallation {
		return fmt.Errorf("invalid runner profile scope type")
	}
	if scope.ID <= 0 {
		return fmt.Errorf("invalid runner profile scope id")
	}
	return nil
}

func NormalizeWorkflowLabels(labels []string) ([]string, string, error) {
	seen := make(map[string]struct{}, len(labels))
	normalized := make([]string, 0, len(labels))
	for _, raw := range labels {
		label := strings.TrimSpace(raw)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		normalized = append(normalized, label)
	}
	sort.Strings(normalized)
	if len(normalized) == 0 {
		return nil, "", fmt.Errorf("workflow labels are required")
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(data)
	return normalized, hex.EncodeToString(sum[:]), nil
}

func (s *DBStore) GetProfileControl(scope RunnerProfileScope, name string) (RunnerProfileControl, error) {
	if err := ValidateRunnerProfileScope(scope); err != nil {
		return RunnerProfileControl{}, err
	}
	db, err := s.dbOrEnsure()
	if err != nil {
		return RunnerProfileControl{}, err
	}
	var record runnerProfileScopeControlRecord
	if err := db.Where("scope_type = ? AND scope_id = ? AND profile_name = ?", scope.Type, scope.ID, strings.TrimSpace(name)).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return RunnerProfileControl{}, ErrNotFound
		}
		return RunnerProfileControl{}, err
	}
	return controlFromRecord(record), nil
}

func (s *DBStore) UpsertProfileControlIfUnchanged(control RunnerProfileControl, expectedUpdatedAt *time.Time) (RunnerProfileControl, error) {
	scope := RunnerProfileScope{Type: control.ScopeType, ID: control.ScopeID}
	if err := ValidateRunnerProfileScope(scope); err != nil {
		return RunnerProfileControl{}, err
	}
	control.ProfileName = strings.TrimSpace(control.ProfileName)
	if control.ProfileName == "" || control.MaxConcurrency < 0 {
		return RunnerProfileControl{}, fmt.Errorf("invalid runner profile control")
	}
	db, err := s.dbOrEnsure()
	if err != nil {
		return RunnerProfileControl{}, err
	}
	global, err := s.GetProfile(control.ProfileName)
	if err != nil {
		return RunnerProfileControl{}, err
	}
	if strings.TrimSpace(global.ManagedBy) == "" {
		return RunnerProfileControl{}, fmt.Errorf("runner profile is not managed")
	}
	now := time.Now().UTC()
	if expectedUpdatedAt != nil && now.Before(expectedUpdatedAt.Add(time.Millisecond)) {
		now = expectedUpdatedAt.Add(time.Millisecond)
	}
	record := runnerProfileScopeControlRecord{ScopeType: scope.Type, ScopeID: scope.ID, ProfileName: control.ProfileName, Enabled: control.Enabled, MaxConcurrency: control.MaxConcurrency, CreatedAt: control.CreatedAt, UpdatedAt: now}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	updates := map[string]any{"enabled": record.Enabled, "max_concurrency": record.MaxConcurrency, "updated_at": record.UpdatedAt}
	var result *gorm.DB
	if expectedUpdatedAt != nil {
		result = db.Model(&runnerProfileScopeControlRecord{}).Where("scope_type = ? AND scope_id = ? AND profile_name = ? AND updated_at = ?", scope.Type, scope.ID, control.ProfileName, *expectedUpdatedAt).Updates(updates)
		if result.Error == nil && result.RowsAffected == 0 {
			var existing runnerProfileScopeControlRecord
			existingErr := db.Where("scope_type = ? AND scope_id = ? AND profile_name = ?", scope.Type, scope.ID, control.ProfileName).First(&existing).Error
			switch {
			case existingErr == nil:
				return RunnerProfileControl{}, ErrConflict
			case !errors.Is(existingErr, gorm.ErrRecordNotFound):
				return RunnerProfileControl{}, existingErr
			}
			var currentGlobal runnerProfileRecord
			if err := db.Where("name = ? AND updated_at = ?", control.ProfileName, *expectedUpdatedAt).First(&currentGlobal).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return RunnerProfileControl{}, ErrConflict
				}
				return RunnerProfileControl{}, err
			}
			result = db.Create(&record)
			if result.Error != nil {
				if translator, ok := db.Dialector.(gorm.ErrorTranslator); ok && errors.Is(translator.Translate(result.Error), gorm.ErrDuplicatedKey) {
					return RunnerProfileControl{}, ErrConflict
				}
				return RunnerProfileControl{}, result.Error
			}
		}
	} else {
		result = db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scope_type"}, {Name: "scope_id"}, {Name: "profile_name"}}, DoUpdates: clause.Assignments(updates)}).Create(&record)
	}
	if result.Error != nil {
		return RunnerProfileControl{}, result.Error
	}
	if expectedUpdatedAt != nil && result.RowsAffected == 0 {
		return RunnerProfileControl{}, ErrConflict
	}
	return s.GetProfileControl(scope, control.ProfileName)
}

func (s *DBStore) DeleteProfileControlIfUnchanged(scope RunnerProfileScope, name string, expectedUpdatedAt *time.Time) error {
	if err := ValidateRunnerProfileScope(scope); err != nil {
		return err
	}
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	query := db.Where("scope_type = ? AND scope_id = ? AND profile_name = ?", scope.Type, scope.ID, strings.TrimSpace(name))
	if expectedUpdatedAt != nil {
		query = query.Where("updated_at = ?", *expectedUpdatedAt)
	}
	result := query.Delete(&runnerProfileScopeControlRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrConflict
	}
	return nil
}

func (s *DBStore) ListScopedProfiles(scope RunnerProfileScope) ([]ScopedRunnerProfile, error) {
	if err := ValidateRunnerProfileScope(scope); err != nil {
		return nil, err
	}
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	var records []scopedRunnerProfileRecord
	if err := db.Where("scope_type = ? AND scope_id = ?", scope.Type, scope.ID).Order("name ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	profiles := make([]ScopedRunnerProfile, 0, len(records))
	for _, record := range records {
		profile, err := scopedProfileFromRecord(record)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func (s *DBStore) GetScopedProfile(scope RunnerProfileScope, name string) (ScopedRunnerProfile, error) {
	if err := ValidateRunnerProfileScope(scope); err != nil {
		return ScopedRunnerProfile{}, err
	}
	db, err := s.dbOrEnsure()
	if err != nil {
		return ScopedRunnerProfile{}, err
	}
	var record scopedRunnerProfileRecord
	if err := db.Where("scope_type = ? AND scope_id = ? AND name = ?", scope.Type, scope.ID, strings.TrimSpace(name)).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ScopedRunnerProfile{}, ErrNotFound
		}
		return ScopedRunnerProfile{}, err
	}
	return scopedProfileFromRecord(record)
}

func (s *DBStore) UpsertScopedProfileIfUnchanged(profile ScopedRunnerProfile, expectedUpdatedAt *time.Time) (ScopedRunnerProfile, error) {
	scope := RunnerProfileScope{Type: profile.ScopeType, ID: profile.ScopeID}
	if err := ValidateRunnerProfileScope(scope); err != nil {
		return ScopedRunnerProfile{}, err
	}
	profile.Name = strings.TrimSpace(profile.Name)
	if profile.Name == "" || strings.Contains(profile.Name, "/") || profile.Name == "." || profile.Name == ".." || profile.MaxConcurrency < 0 {
		return ScopedRunnerProfile{}, fmt.Errorf("invalid scoped runner profile")
	}
	if global, globalErr := s.GetProfile(profile.Name); globalErr == nil && global.Enabled {
		return ScopedRunnerProfile{}, ErrRunnerProfileNameConflict
	} else if globalErr != nil && !errors.Is(globalErr, ErrNotFound) {
		return ScopedRunnerProfile{}, globalErr
	}
	labels, labelKey, err := NormalizeWorkflowLabels(profile.WorkflowLabels)
	if err != nil {
		return ScopedRunnerProfile{}, err
	}
	profile.WorkflowLabels, profile.LabelKey = labels, labelKey
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return ScopedRunnerProfile{}, err
	}
	db, err := s.dbOrEnsure()
	if err != nil {
		return ScopedRunnerProfile{}, err
	}
	now := time.Now().UTC()
	if expectedUpdatedAt != nil && now.Before(expectedUpdatedAt.Add(time.Millisecond)) {
		now = expectedUpdatedAt.Add(time.Millisecond)
	}
	record := scopedRunnerProfileRecord{ScopeType: scope.Type, ScopeID: scope.ID, Name: profile.Name, WorkflowLabelsJSON: string(labelsJSON), LabelKey: labelKey, TemplateID: strings.TrimSpace(profile.TemplateID), RunnerGroup: strings.TrimSpace(profile.RunnerGroup), MaxConcurrency: profile.MaxConcurrency, Enabled: profile.Enabled, CreatedAt: profile.CreatedAt, UpdatedAt: now}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	updates := map[string]any{"workflow_labels_json": record.WorkflowLabelsJSON, "label_key": record.LabelKey, "template_id": record.TemplateID, "runner_group": record.RunnerGroup, "max_concurrency": record.MaxConcurrency, "enabled": record.Enabled, "updated_at": record.UpdatedAt}
	var result *gorm.DB
	if expectedUpdatedAt != nil {
		result = db.Model(&scopedRunnerProfileRecord{}).Where("scope_type = ? AND scope_id = ? AND name = ? AND updated_at = ?", scope.Type, scope.ID, profile.Name, *expectedUpdatedAt).Updates(updates)
	} else {
		result = db.Create(&record)
		if result.Error != nil {
			if translator, ok := db.Dialector.(gorm.ErrorTranslator); ok && errors.Is(translator.Translate(result.Error), gorm.ErrDuplicatedKey) {
				var existing scopedRunnerProfileRecord
				existingErr := db.Where("scope_type = ? AND scope_id = ? AND name = ?", scope.Type, scope.ID, profile.Name).First(&existing).Error
				if existingErr == nil {
					return ScopedRunnerProfile{}, ErrConflict
				}
				if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
					return ScopedRunnerProfile{}, existingErr
				}
				return ScopedRunnerProfile{}, ErrRunnerProfileLabelsConflict
			}
		}
	}
	if result.Error != nil {
		return ScopedRunnerProfile{}, result.Error
	}
	if expectedUpdatedAt != nil && result.RowsAffected == 0 {
		return ScopedRunnerProfile{}, ErrConflict
	}
	return s.GetScopedProfile(scope, profile.Name)
}

func (s *DBStore) DeleteScopedProfileIfUnchanged(scope RunnerProfileScope, name string, expectedUpdatedAt *time.Time) error {
	if err := ValidateRunnerProfileScope(scope); err != nil {
		return err
	}
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	query := db.Where("scope_type = ? AND scope_id = ? AND name = ?", scope.Type, scope.ID, strings.TrimSpace(name))
	if expectedUpdatedAt != nil {
		query = query.Where("updated_at = ?", *expectedUpdatedAt)
	}
	result := query.Delete(&scopedRunnerProfileRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrConflict
	}
	return nil
}

func (s *DBStore) ListEffectiveProfiles(scope RunnerProfileScope) ([]EffectiveRunnerProfile, error) {
	if err := ValidateRunnerProfileScope(scope); err != nil {
		return nil, err
	}
	globals, err := s.ListProfiles()
	if err != nil {
		return nil, err
	}
	controls, err := s.GetProfileControls(scope)
	if err != nil {
		return nil, err
	}
	controlMap := make(map[string]RunnerProfileControl, len(controls))
	for _, control := range controls {
		controlMap[control.ProfileName] = control
	}
	items := make([]EffectiveRunnerProfile, 0, len(globals))
	for _, profile := range globals {
		control := controlMap[profile.Name]
		scopeLimit := 0
		enabled := profile.Enabled
		updatedAt := profile.UpdatedAt
		if control.ProfileName != "" {
			scopeLimit, enabled = control.MaxConcurrency, enabled && control.Enabled
			updatedAt = control.UpdatedAt
		}
		source := "platform_custom"
		if profile.ManagedBy != "" {
			source = "managed"
		}
		profile.UpdatedAt = updatedAt
		items = append(items, EffectiveRunnerProfile{Source: source, ScopeType: scope.Type, ScopeID: scope.ID, Profile: profile, WorkflowLabels: append([]string(nil), profile.Labels...), GlobalMaxConcurrency: profile.MaxConcurrency, ScopeMaxConcurrency: scopeLimit, EffectiveEnabled: enabled, Editable: profile.ManagedBy != "", ScopeControlConfigured: control.ProfileName != ""})
	}
	scoped, err := s.ListScopedProfiles(scope)
	if err != nil {
		return nil, err
	}
	for _, profile := range scoped {
		items = append(items, EffectiveRunnerProfile{Source: "scoped_custom", ScopeType: scope.Type, ScopeID: scope.ID, Profile: RunnerProfile{Name: profile.Name, Labels: append([]string(nil), profile.WorkflowLabels...), RequiredLabels: append([]string(nil), profile.WorkflowLabels...), TemplateID: profile.TemplateID, RunnerGroup: profile.RunnerGroup, MaxConcurrency: profile.MaxConcurrency, Enabled: profile.Enabled, CreatedAt: profile.CreatedAt, UpdatedAt: profile.UpdatedAt}, WorkflowLabels: append([]string(nil), profile.WorkflowLabels...), ScopeMaxConcurrency: profile.MaxConcurrency, EffectiveEnabled: profile.Enabled, OverridesGlobal: true, Editable: true})
	}
	return items, nil
}

func (s *DBStore) GetEffectiveProfile(scope RunnerProfileScope, source, name string) (EffectiveRunnerProfile, error) {
	items, err := s.ListEffectiveProfiles(scope)
	if err != nil {
		return EffectiveRunnerProfile{}, err
	}
	for _, item := range items {
		globalMatch := source == "global" && item.Source != "scoped_custom"
		if (item.Source == source || globalMatch) && item.Profile.Name == strings.TrimSpace(name) {
			return item, nil
		}
	}
	return EffectiveRunnerProfile{}, ErrNotFound
}

func (s *DBStore) MatchProfileForScope(scope RunnerProfileScope, repositoryFullName string, labels []string) (ProfileMatch, error) {
	if err := ValidateRunnerProfileScope(scope); err != nil {
		return ProfileMatch{}, err
	}
	match := ProfileMatch{RepositoryFullName: repositoryFullName, Labels: append([]string(nil), labels...), ScopeType: scope.Type, ScopeID: scope.ID}
	scoped, err := s.ListScopedProfiles(scope)
	if err != nil {
		return ProfileMatch{}, err
	}
	_, labelKey, normalizeErr := NormalizeWorkflowLabels(labels)
	if normalizeErr == nil {
		for _, profile := range scoped {
			if profile.LabelKey != labelKey {
				continue
			}
			match.Source = "scoped_custom"
			if !profile.Enabled {
				match.Reason = "profile_scope_disabled"
				return match, nil
			}
			runnerProfile := RunnerProfile{Name: profile.Name, Labels: append([]string(nil), profile.WorkflowLabels...), RequiredLabels: append([]string(nil), profile.WorkflowLabels...), TemplateID: profile.TemplateID, RunnerGroup: profile.RunnerGroup, MaxConcurrency: profile.MaxConcurrency, Enabled: true, CreatedAt: profile.CreatedAt, UpdatedAt: profile.UpdatedAt}
			match.Profile = &runnerProfile
			return match, nil
		}
	}
	globals, err := s.ListProfiles()
	if err != nil {
		return ProfileMatch{}, err
	}
	controls, err := s.GetProfileControls(scope)
	if err != nil {
		return ProfileMatch{}, err
	}
	controlMap := make(map[string]RunnerProfileControl, len(controls))
	for _, control := range controls {
		controlMap[control.ProfileName] = control
	}
	for i := range globals {
		if globals[i].ManagedBy == "" {
			continue
		}
		if control, ok := controlMap[globals[i].Name]; ok {
			globals[i].Enabled = globals[i].Enabled && control.Enabled
		}
	}
	match = profileMatchFromCandidates(repositoryFullName, labels, globals)
	match.Source, match.ScopeType, match.ScopeID = "global", scope.Type, scope.ID
	return match, nil
}

func (s *DBStore) GetProfileControls(scope RunnerProfileScope) ([]RunnerProfileControl, error) {
	if err := ValidateRunnerProfileScope(scope); err != nil {
		return nil, err
	}
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	var records []runnerProfileScopeControlRecord
	if err := db.Where("scope_type = ? AND scope_id = ?", scope.Type, scope.ID).Order("profile_name ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	controls := make([]RunnerProfileControl, 0, len(records))
	for _, record := range records {
		controls = append(controls, controlFromRecord(record))
	}
	return controls, nil
}

func controlFromRecord(record runnerProfileScopeControlRecord) RunnerProfileControl {
	return RunnerProfileControl{ScopeType: record.ScopeType, ScopeID: record.ScopeID, ProfileName: record.ProfileName, Enabled: record.Enabled, MaxConcurrency: record.MaxConcurrency, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}

func scopedProfileFromRecord(record scopedRunnerProfileRecord) (ScopedRunnerProfile, error) {
	var labels []string
	if err := json.Unmarshal([]byte(record.WorkflowLabelsJSON), &labels); err != nil {
		return ScopedRunnerProfile{}, err
	}
	return ScopedRunnerProfile{ScopeType: record.ScopeType, ScopeID: record.ScopeID, Name: record.Name, WorkflowLabels: labels, LabelKey: record.LabelKey, TemplateID: record.TemplateID, RunnerGroup: record.RunnerGroup, MaxConcurrency: record.MaxConcurrency, Enabled: record.Enabled, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

func (s *DBStore) ActiveCountForProfileScope(source string, scope RunnerProfileScope, name string) (int, error) {
	return s.countStatesForProfileScope(source, scope, name, []string{StatusQueued, StatusCreating, StatusRunning, StatusStopping})
}

func (s *DBStore) InFlightCountForProfileScope(source string, scope RunnerProfileScope, name string) (int, error) {
	return s.countStatesForProfileScope(source, scope, name, []string{StatusCreating, StatusRunning, StatusStopping})
}

func (s *DBStore) countStatesForProfileScope(source string, scope RunnerProfileScope, name string, statuses []string) (int, error) {
	if err := ValidateRunnerProfileScope(scope); err != nil {
		return 0, err
	}
	db, err := s.dbOrEnsure()
	if err != nil {
		return 0, err
	}
	var count int64
	if err := db.Model(&runnerRequestRecord{}).Where("profile_source = ? AND profile_scope_type = ? AND profile_scope_id = ? AND profile_name = ? AND status IN ?", source, scope.Type, scope.ID, strings.TrimSpace(name), statuses).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}
