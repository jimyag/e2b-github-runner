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
	if err := validateProfileName(profile.Name); err != nil {
		return RunnerProfile{}, err
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
		DefaultAvailable:    true,
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
			if err := validateProfileName(profile.Name); err != nil {
				return err
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
					DefaultAvailable:    true,
					ManagedBy:           profile.ManagedBy,
					CatalogRevision:     profile.CatalogRevision,
					CreatedAt:           createdAt,
					UpdatedAt:           now,
				}
				if err := tx.Create(&record).Error; err != nil {
					return err
				}
				if err := appendManagedProfileReconciliationAudit(tx, profile, now); err != nil {
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
				continue
			}
			if err := appendManagedProfileReconciliationAudit(tx, profile, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return conflicts, nil
}

func validateProfileName(name string) error {
	if strings.Contains(name, "/") || name == "." || name == ".." {
		return fmt.Errorf("profile name must not contain '/' or be '.' or '..'")
	}
	return nil
}

func appendManagedProfileReconciliationAudit(tx *gorm.DB, profile RunnerProfile, now time.Time) error {
	payload, err := json.Marshal(map[string]any{
		"managed_by":       profile.ManagedBy,
		"catalog_revision": profile.CatalogRevision,
	})
	if err != nil {
		return err
	}
	return tx.Create(&auditEventRecord{
		Actor:        "runnerd_startup",
		Action:       "profile.reconcile",
		ResourceType: "runner_profile",
		ResourceID:   profile.Name,
		PayloadJSON:  string(payload),
		CreatedAt:    now,
	}).Error
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
	return db.Delete(&runnerProfileRecord{}, "name = ?", name).Error
}

func (s *DBStore) MatchProfile(repositoryFullName string, labels []string) (ProfileMatch, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return ProfileMatch{}, err
	}
	profiles, err := loadRunnerProfilesForMatch(db)
	if err != nil {
		return ProfileMatch{}, err
	}
	return profileMatchFromCandidates(repositoryFullName, labels, profiles), nil
}

func loadRunnerProfilesForMatch(tx *gorm.DB) ([]RunnerProfile, error) {
	var profileRecords []runnerProfileRecord
	if err := tx.Order("priority DESC, name ASC").Find(&profileRecords).Error; err != nil {
		return nil, err
	}
	profiles := make([]RunnerProfile, 0, len(profileRecords))
	for _, record := range profileRecords {
		profile, err := recordToProfile(record)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
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
