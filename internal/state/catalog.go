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
	labelsJSON, err := json.Marshal(profile.Labels)
	if err != nil {
		return RunnerProfile{}, err
	}
	now := time.Now().UTC()
	record := runnerProfileRecord{
		Name:             profile.Name,
		LabelsJSON:       string(labelsJSON),
		TemplateID:       profile.TemplateID,
		RunnerGroup:      profile.RunnerGroup,
		MaxConcurrency:   profile.MaxConcurrency,
		MinIdle:          profile.MinIdle,
		Priority:         profile.Priority,
		Enabled:          profile.Enabled,
		DefaultAvailable: profile.DefaultAvailable,
		CreatedAt:        profile.CreatedAt,
		UpdatedAt:        now,
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.Assignments(map[string]any{"labels_json": record.LabelsJSON, "template_id": record.TemplateID, "runner_group": record.RunnerGroup, "max_concurrency": record.MaxConcurrency, "min_idle": record.MinIdle, "priority": record.Priority, "enabled": record.Enabled, "default_available": record.DefaultAvailable, "updated_at": record.UpdatedAt}),
	}).Create(&record).Error; err != nil {
		return RunnerProfile{}, err
	}
	return s.GetProfile(record.Name)
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
	policies, err := s.ListRepositoryPolicies()
	if err != nil {
		return ProfileMatch{}, err
	}
	profiles, err := s.ListProfiles()
	if err != nil {
		return ProfileMatch{}, err
	}
	groups, err := s.ListRunnerGroups()
	if err != nil {
		return ProfileMatch{}, err
	}
	groupsByName := make(map[string]RunnerGroup, len(groups))
	for _, group := range groups {
		groupsByName[group.Name] = group
	}
	match := ProfileMatch{
		RepositoryFullName: repositoryFullName,
		Labels:             append([]string(nil), labels...),
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
	if len(allowed) == 0 {
		match.Reason = "profile_not_allowed"
		return match, nil
	}
	var candidates []RunnerProfile
	for _, profile := range profiles {
		if !profile.Enabled || !allowed[profile.Name] {
			continue
		}
		if labelsMatch(labels, profile.Labels) {
			candidates = append(candidates, profile)
		}
	}
	if len(candidates) == 0 {
		match.Reason = "profile_labels_not_matched"
		return match, nil
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
	match.Profile = &candidates[0]
	return match, nil
}
