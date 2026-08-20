package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *DBStore) ListProfiles() ([]RunnerProfile, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	var records []runnerProfileRecord
	if err := db.Order("priority DESC, name ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	profiles := make([]RunnerProfile, 0, len(records))
	for _, record := range records {
		profile, err := recordToProfile(record)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func (s *DBStore) GetProfile(name string) (RunnerProfile, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RunnerProfile{}, err
	}
	var record runnerProfileRecord
	if err := db.First(&record, "name = ?", strings.TrimSpace(name)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return RunnerProfile{}, ErrNotFound
		}
		return RunnerProfile{}, err
	}
	return recordToProfile(record)
}

func (s *DBStore) UpsertProfile(profile RunnerProfile) (RunnerProfile, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RunnerProfile{}, err
	}
	profile.Name = strings.TrimSpace(profile.Name)
	if profile.Name == "" {
		return RunnerProfile{}, fmt.Errorf("profile name is required")
	}
	if !labelsMatch(profile.RequiredLabels, profile.Labels) {
		return RunnerProfile{}, fmt.Errorf("required labels must be a subset of labels")
	}
	labelsJSON, err := json.Marshal(profile.Labels)
	if err != nil {
		return RunnerProfile{}, err
	}
	if profile.RequiredLabels == nil {
		profile.RequiredLabels = []string{}
	}
	requiredLabelsJSON, err := json.Marshal(profile.RequiredLabels)
	if err != nil {
		return RunnerProfile{}, err
	}
	requiredLabelsJSONText := string(requiredLabelsJSON)
	now := time.Now().UTC()
	record := runnerProfileRecord{
		Name:                profile.Name,
		LabelsJSON:          string(labelsJSON),
		RequiredLabelsJSON:  &requiredLabelsJSONText,
		TemplateID:          profile.TemplateID,
		DefaultTemplateName: profile.DefaultTemplateName,
		RunnerGroup:         profile.RunnerGroup,
		MaxConcurrency:      profile.MaxConcurrency,
		MinIdle:             profile.MinIdle,
		Priority:            profile.Priority,
		Enabled:             profile.Enabled,
		DefaultAvailable:    profile.DefaultAvailable,
		ManagedBy:           profile.ManagedBy,
		CatalogRevision:     profile.CatalogRevision,
		CreatedAt:           profile.CreatedAt,
		UpdatedAt:           now,
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "name"}},
		DoUpdates: clause.Assignments(map[string]any{
			"labels_json":           record.LabelsJSON,
			"required_labels_json":  record.RequiredLabelsJSON,
			"template_id":           record.TemplateID,
			"default_template_name": record.DefaultTemplateName,
			"runner_group":          record.RunnerGroup,
			"max_concurrency":       record.MaxConcurrency,
			"min_idle":              record.MinIdle,
			"priority":              record.Priority,
			"enabled":               record.Enabled,
			"default_available":     record.DefaultAvailable,
			"managed_by":            record.ManagedBy,
			"catalog_revision":      record.CatalogRevision,
			"updated_at":            record.UpdatedAt,
		}),
	}).Create(&record).Error; err != nil {
		return RunnerProfile{}, err
	}
	return s.GetProfile(record.Name)
}

// ReconcileManagedProfiles creates or upgrades catalog-owned profiles while preserving operator controls.
func (s *DBStore) ReconcileManagedProfiles(profiles []RunnerProfile) ([]ManagedProfileConflict, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	conflicts := make([]ManagedProfileConflict, 0)
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, profile := range profiles {
			profile.Name = strings.TrimSpace(profile.Name)
			profile.ManagedBy = strings.TrimSpace(profile.ManagedBy)
			if profile.Name == "" {
				return fmt.Errorf("profile name is required")
			}
			if profile.ManagedBy == "" {
				return fmt.Errorf("managed profile owner is required")
			}
			if !labelsMatch(profile.RequiredLabels, profile.Labels) {
				return fmt.Errorf("required labels must be a subset of labels")
			}

			var existing runnerProfileRecord
			findErr := tx.First(&existing, "name = ?", profile.Name).Error
			if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
				return findErr
			}
			if findErr == nil {
				if existing.ManagedBy == "" || existing.ManagedBy != profile.ManagedBy {
					conflicts = append(conflicts, ManagedProfileConflict{
						Name:              profile.Name,
						ExistingManagedBy: existing.ManagedBy,
					})
					continue
				}
				if existing.CatalogRevision >= profile.CatalogRevision {
					continue
				}
			}

			labelsJSON, requiredLabelsJSON, err := marshalProfileLabels(profile)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			if errors.Is(findErr, gorm.ErrRecordNotFound) {
				createdAt := profile.CreatedAt
				if createdAt.IsZero() {
					createdAt = now
				}
				record := runnerProfileRecord{
					Name:                profile.Name,
					LabelsJSON:          labelsJSON,
					RequiredLabelsJSON:  &requiredLabelsJSON,
					TemplateID:          profile.TemplateID,
					DefaultTemplateName: profile.DefaultTemplateName,
					RunnerGroup:         profile.RunnerGroup,
					MaxConcurrency:      profile.MaxConcurrency,
					MinIdle:             profile.MinIdle,
					Priority:            profile.Priority,
					Enabled:             profile.Enabled,
					DefaultAvailable:    profile.DefaultAvailable,
					ManagedBy:           profile.ManagedBy,
					CatalogRevision:     profile.CatalogRevision,
					CreatedAt:           createdAt,
					UpdatedAt:           now,
				}
				if err := tx.Create(&record).Error; err != nil {
					return err
				}
				continue
			}

			updates := map[string]any{
				"labels_json":           labelsJSON,
				"required_labels_json":  &requiredLabelsJSON,
				"template_id":           profile.TemplateID,
				"default_template_name": profile.DefaultTemplateName,
				"runner_group":          profile.RunnerGroup,
				"priority":              profile.Priority,
				"default_available":     profile.DefaultAvailable,
				"managed_by":            profile.ManagedBy,
				"catalog_revision":      profile.CatalogRevision,
				"updated_at":            now,
			}
			result := tx.Model(&runnerProfileRecord{}).
				Where(
					"name = ? AND managed_by = ? AND catalog_revision < ?",
					profile.Name,
					existing.ManagedBy,
					profile.CatalogRevision,
				).
				Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				var current runnerProfileRecord
				if err := tx.First(&current, "name = ?", profile.Name).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						continue
					}
					return err
				}
				if current.ManagedBy == "" || current.ManagedBy != profile.ManagedBy {
					conflicts = append(conflicts, ManagedProfileConflict{
						Name:              profile.Name,
						ExistingManagedBy: current.ManagedBy,
					})
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return conflicts, nil
}

func marshalProfileLabels(profile RunnerProfile) (string, string, error) {
	labelsJSON, err := json.Marshal(profile.Labels)
	if err != nil {
		return "", "", err
	}
	requiredLabels := profile.RequiredLabels
	if requiredLabels == nil {
		requiredLabels = []string{}
	}
	requiredLabelsJSON, err := json.Marshal(requiredLabels)
	if err != nil {
		return "", "", err
	}
	return string(labelsJSON), string(requiredLabelsJSON), nil
}

func (s *DBStore) DeleteProfile(name string) error {
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&runnerGroupSpecRecord{}, "spec_name = ?", name).Error; err != nil {
			return err
		}
		return tx.Delete(&runnerProfileRecord{}, "name = ?", name).Error
	})
}

func (s *DBStore) ListRunnerGroups() ([]RunnerGroup, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	var records []runnerGroupRecord
	if err := db.Order("name ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	groups := make([]RunnerGroup, 0, len(records))
	for _, record := range records {
		group, err := s.recordToRunnerGroup(db, record)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func (s *DBStore) GetRunnerGroup(name string) (RunnerGroup, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RunnerGroup{}, err
	}
	var record runnerGroupRecord
	if err := db.First(&record, "name = ?", strings.TrimSpace(name)).Error; err != nil {
		return RunnerGroup{}, err
	}
	return s.recordToRunnerGroup(db, record)
}

func (s *DBStore) UpsertRunnerGroup(group RunnerGroup) (RunnerGroup, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RunnerGroup{}, err
	}
	group.Name = strings.TrimSpace(group.Name)
	if group.Name == "" {
		return RunnerGroup{}, fmt.Errorf("runner group name is required")
	}
	specNames := uniqueTrimmed(group.SpecNames)
	now := time.Now().UTC()
	record := runnerGroupRecord{
		Name:        group.Name,
		Description: strings.TrimSpace(group.Description),
		Enabled:     group.Enabled,
		CreatedAt:   group.CreatedAt,
		UpdatedAt:   now,
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if len(specNames) > 0 {
			var existingNames []string
			if err := tx.Model(&runnerProfileRecord{}).Where("name IN ?", specNames).Pluck("name", &existingNames).Error; err != nil {
				return err
			}
			existing := make(map[string]bool, len(existingNames))
			for _, name := range existingNames {
				existing[name] = true
			}
			for _, specName := range specNames {
				if !existing[specName] {
					return fmt.Errorf("runner spec %q does not exist", specName)
				}
			}
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "name"}},
			DoUpdates: clause.Assignments(map[string]any{
				"description": record.Description,
				"enabled":     record.Enabled,
				"updated_at":  record.UpdatedAt,
			}),
		}).Create(&record).Error; err != nil {
			return err
		}
		if err := tx.Delete(&runnerGroupSpecRecord{}, "group_name = ?", record.Name).Error; err != nil {
			return err
		}
		for _, specName := range specNames {
			link := runnerGroupSpecRecord{GroupName: record.Name, SpecName: specName, CreatedAt: now}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return RunnerGroup{}, err
	}
	return s.GetRunnerGroup(record.Name)
}

func (s *DBStore) DeleteRunnerGroup(name string) error {
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&runnerGroupSpecRecord{}, "group_name = ?", name).Error; err != nil {
			return err
		}
		if err := tx.Model(&repositoryPolicyRecord{}).
			Where("runner_group_name = ?", name).
			Update("enabled", false).Error; err != nil {
			return err
		}
		return tx.Delete(&runnerGroupRecord{}, "name = ?", name).Error
	})
}

func (s *DBStore) ListRepositoryPolicies() ([]RepositoryPolicy, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	var records []repositoryPolicyRecord
	if err := db.Order("repository_full_name ASC, profile_name ASC, runner_group_name ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	policies := make([]RepositoryPolicy, 0, len(records))
	for _, record := range records {
		policies = append(policies, recordToRepositoryPolicy(record))
	}
	return policies, nil
}

func (s *DBStore) GetRepositoryPolicy(id int64) (RepositoryPolicy, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RepositoryPolicy{}, err
	}
	var record repositoryPolicyRecord
	if err := db.First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return RepositoryPolicy{}, ErrNotFound
		}
		return RepositoryPolicy{}, err
	}
	return recordToRepositoryPolicy(record), nil
}

func (s *DBStore) UpsertRepositoryPolicy(policy RepositoryPolicy) (RepositoryPolicy, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return RepositoryPolicy{}, err
	}
	policy.RepositoryFullName = strings.TrimSpace(policy.RepositoryFullName)
	policy.ProfileName = strings.TrimSpace(policy.ProfileName)
	policy.RunnerGroupName = strings.TrimSpace(policy.RunnerGroupName)
	if policy.RepositoryFullName == "" {
		return RepositoryPolicy{}, fmt.Errorf("repository_full_name is required")
	}
	if (policy.ProfileName == "") == (policy.RunnerGroupName == "") {
		return RepositoryPolicy{}, fmt.Errorf("exactly one of runner_spec_name or runner_group_name is required")
	}
	now := time.Now().UTC()
	if policy.ID == 0 {
		record := repositoryPolicyRecord{
			RepositoryFullName: policy.RepositoryFullName,
			ProfileName:        policy.ProfileName,
			RunnerGroupName:    policy.RunnerGroupName,
			Enabled:            policy.Enabled,
			CreatedAt:          now,
		}
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "repository_full_name"},
				{Name: "profile_name"},
				{Name: "runner_group_name"},
			},
			DoUpdates: clause.Assignments(map[string]any{
				"enabled": record.Enabled,
			}),
		}).Create(&record).Error; err != nil {
			return RepositoryPolicy{}, err
		}
		var saved repositoryPolicyRecord
		if err := db.First(&saved, "repository_full_name = ? AND profile_name = ? AND runner_group_name = ?", record.RepositoryFullName, record.ProfileName, record.RunnerGroupName).Error; err != nil {
			return RepositoryPolicy{}, err
		}
		return recordToRepositoryPolicy(saved), nil
	}
	updates := map[string]any{
		"repository_full_name": policy.RepositoryFullName,
		"profile_name":         policy.ProfileName,
		"runner_group_name":    policy.RunnerGroupName,
		"enabled":              policy.Enabled,
	}
	if err := db.Model(&repositoryPolicyRecord{}).Where("id = ?", policy.ID).Updates(updates).Error; err != nil {
		return RepositoryPolicy{}, err
	}
	var saved repositoryPolicyRecord
	if err := db.First(&saved, "id = ?", policy.ID).Error; err != nil {
		return RepositoryPolicy{}, err
	}
	return recordToRepositoryPolicy(saved), nil
}

func (s *DBStore) DeleteRepositoryPolicy(id int64) error {
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	return db.Delete(&repositoryPolicyRecord{}, "id = ?", id).Error
}

func (s *DBStore) MatchProfile(repositoryFullName string, labels []string) (ProfileMatch, error) {
	comparison, err := s.CompareProfileMatches(repositoryFullName, labels)
	return comparison.Legacy, err
}

func (s *DBStore) CompareProfileMatches(repositoryFullName string, labels []string) (ProfileMatchComparison, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return ProfileMatchComparison{}, err
	}
	var snapshot catalogMatchSnapshot
	err = db.Transaction(func(tx *gorm.DB) error {
		var loadErr error
		snapshot, loadErr = loadCatalogMatchSnapshot(tx)
		return loadErr
	}, catalogSnapshotTxOptions)
	if err != nil {
		return ProfileMatchComparison{}, err
	}
	return snapshot.compare(repositoryFullName, labels), nil
}

type catalogMatchSnapshot struct {
	profiles []RunnerProfile
	policies []RepositoryPolicy
	groups   []RunnerGroup
}

func loadCatalogMatchSnapshot(tx *gorm.DB) (catalogMatchSnapshot, error) {
	var snapshot catalogMatchSnapshot
	var profileRecords []runnerProfileRecord
	if err := tx.Order("priority DESC, name ASC").Find(&profileRecords).Error; err != nil {
		return catalogMatchSnapshot{}, err
	}
	snapshot.profiles = make([]RunnerProfile, 0, len(profileRecords))
	for _, record := range profileRecords {
		profile, err := recordToProfile(record)
		if err != nil {
			return catalogMatchSnapshot{}, err
		}
		snapshot.profiles = append(snapshot.profiles, profile)
	}

	hasPolicies, err := catalogTableExists(tx, repositoryPolicyRecord{}.TableName())
	if err != nil {
		return catalogMatchSnapshot{}, err
	}
	if hasPolicies {
		var policyRecords []repositoryPolicyRecord
		if err := tx.Order("repository_full_name ASC, id ASC").Find(&policyRecords).Error; err != nil {
			return catalogMatchSnapshot{}, err
		}
		snapshot.policies = make([]RepositoryPolicy, 0, len(policyRecords))
		for _, record := range policyRecords {
			snapshot.policies = append(snapshot.policies, recordToRepositoryPolicy(record))
		}
	}

	hasGroups, err := catalogTableExists(tx, runnerGroupRecord{}.TableName())
	if err != nil {
		return catalogMatchSnapshot{}, err
	}
	if hasGroups {
		var groupRecords []runnerGroupRecord
		if err := tx.Order("name ASC").Find(&groupRecords).Error; err != nil {
			return catalogMatchSnapshot{}, err
		}
		specsByGroup := make(map[string][]string, len(groupRecords))
		hasGroupSpecs, err := catalogTableExists(tx, runnerGroupSpecRecord{}.TableName())
		if err != nil {
			return catalogMatchSnapshot{}, err
		}
		if hasGroupSpecs {
			var specRecords []runnerGroupSpecRecord
			if err := tx.Order("group_name ASC, spec_name ASC").Find(&specRecords).Error; err != nil {
				return catalogMatchSnapshot{}, err
			}
			for _, record := range specRecords {
				specsByGroup[record.GroupName] = append(specsByGroup[record.GroupName], record.SpecName)
			}
		}
		snapshot.groups = make([]RunnerGroup, 0, len(groupRecords))
		for _, record := range groupRecords {
			snapshot.groups = append(snapshot.groups, RunnerGroup{
				Name: record.Name, Description: record.Description,
				SpecNames: specsByGroup[record.Name], Enabled: record.Enabled,
				CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
			})
		}
	}
	return snapshot, nil
}

func (snapshot catalogMatchSnapshot) compare(repositoryFullName string, labels []string) ProfileMatchComparison {
	legacyCandidates, legacyHasAllowedNames := legacyAllowedProfiles(snapshot.profiles, snapshot.policies, snapshot.groups, repositoryFullName)
	legacy := profileMatchFromCandidates(repositoryFullName, labels, legacyCandidates)
	if !legacyHasAllowedNames {
		legacy.Reason = "profile_not_allowed"
	}
	enabled := profileMatchFromCandidates(repositoryFullName, labels, snapshot.profiles)
	return ProfileMatchComparison{Legacy: legacy, Enabled: enabled}
}

func catalogTableExists(tx *gorm.DB, tableName string) (bool, error) {
	var query string
	switch tx.Dialector.Name() {
	case BackendSQLite:
		query = `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`
	case BackendPostgres:
		query = `SELECT count(*) FROM information_schema.tables WHERE table_schema = CURRENT_SCHEMA() AND table_name = ? AND table_type = 'BASE TABLE'`
	case BackendMySQL:
		query = `SELECT count(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ? AND table_type = 'BASE TABLE'`
	default:
		return false, fmt.Errorf("unsupported state backend for catalog table lookup: %s", tx.Dialector.Name())
	}
	var count int64
	if err := tx.Raw(query, tableName).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("check catalog table %q: %w", tableName, err)
	}
	return count > 0, nil
}

func legacyAllowedProfiles(profiles []RunnerProfile, policies []RepositoryPolicy, groups []RunnerGroup, repositoryFullName string) ([]RunnerProfile, bool) {
	groupsByName := make(map[string]RunnerGroup, len(groups))
	for _, group := range groups {
		groupsByName[group.Name] = group
	}
	allowed := map[string]bool{}
	for _, profile := range profiles {
		if profile.Enabled && profile.DefaultAvailable {
			allowed[profile.Name] = true
		}
	}
	for _, policy := range policies {
		if !policy.Enabled {
			continue
		}
		if repositoryMatches(policy.RepositoryFullName, repositoryFullName) {
			if policy.ProfileName != "" {
				allowed[policy.ProfileName] = true
			}
			if policy.RunnerGroupName != "" {
				group := groupsByName[policy.RunnerGroupName]
				if !group.Enabled {
					continue
				}
				for _, specName := range group.SpecNames {
					allowed[specName] = true
				}
			}
		}
	}
	var candidates []RunnerProfile
	for _, profile := range profiles {
		if allowed[profile.Name] {
			candidates = append(candidates, profile)
		}
	}
	return candidates, len(allowed) > 0
}

func profileMatchFromCandidates(repositoryFullName string, labels []string, profiles []RunnerProfile) ProfileMatch {
	match := ProfileMatch{RepositoryFullName: repositoryFullName, Labels: append([]string(nil), labels...)}
	match.Profile = selectMatchingProfile(profiles, labels)
	if match.Profile == nil {
		match.Reason = "profile_labels_not_matched"
	}
	return match
}

func selectMatchingProfile(profiles []RunnerProfile, labels []string) *RunnerProfile {
	var candidates []RunnerProfile
	for _, profile := range profiles {
		if profile.Enabled && labelsMatch(profile.RequiredLabels, labels) && labelsMatch(labels, profile.Labels) {
			candidates = append(candidates, profile)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		if len(candidates[i].Labels) != len(candidates[j].Labels) {
			return len(candidates[i].Labels) > len(candidates[j].Labels)
		}
		return candidates[i].Name < candidates[j].Name
	})
	selected := candidates[0]
	return &selected
}
