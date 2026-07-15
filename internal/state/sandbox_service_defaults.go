package state

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const sandboxServiceDefaultID int64 = 1

func (s *DBStore) GetSandboxServiceDefault() (SandboxServiceDefault, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return SandboxServiceDefault{}, err
	}
	var record sandboxServiceDefaultRecord
	if err := db.First(&record, "id = ?", sandboxServiceDefaultID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return SandboxServiceDefault{}, ErrNotFound
		}
		return SandboxServiceDefault{}, err
	}
	return recordToSandboxServiceDefault(record), nil
}

func (s *DBStore) UpsertSandboxServiceDefault(defaultConfig SandboxServiceDefault) (SandboxServiceDefault, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return SandboxServiceDefault{}, err
	}
	now := time.Now().UTC()
	record := sandboxServiceDefaultRecord{
		ID:              sandboxServiceDefaultID,
		Enabled:         defaultConfig.Enabled,
		AudienceMode:    normalizeSandboxServiceDefaultAudienceMode(defaultConfig.AudienceMode),
		APIURL:          strings.TrimSpace(defaultConfig.APIURL),
		APIKeyEncrypted: strings.TrimSpace(defaultConfig.APIKeyEncrypted),
		APIKeyUpdatedAt: defaultConfig.APIKeyUpdatedAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	updateColumns := []string{"enabled", "audience_mode", "api_url", "updated_at"}
	if record.APIKeyEncrypted != "" {
		if record.APIKeyUpdatedAt == nil {
			record.APIKeyUpdatedAt = &now
		}
		updateColumns = append(updateColumns, "api_key_encrypted", "api_key_updated_at")
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns(updateColumns),
	}).Create(&record).Error; err != nil {
		return SandboxServiceDefault{}, err
	}
	if err := db.First(&record, "id = ?", sandboxServiceDefaultID).Error; err != nil {
		return SandboxServiceDefault{}, err
	}
	return recordToSandboxServiceDefault(record), nil
}

func (s *DBStore) UpdateSandboxServiceDefaultPreservingAPIKey(defaultConfig SandboxServiceDefault) (SandboxServiceDefault, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return SandboxServiceDefault{}, err
	}
	now := time.Now().UTC()
	result := db.Model(&sandboxServiceDefaultRecord{}).
		Where("id = ? AND api_key_encrypted IS NOT NULL AND api_key_encrypted != ''", sandboxServiceDefaultID).
		Updates(map[string]any{
			"enabled":       defaultConfig.Enabled,
			"audience_mode": normalizeSandboxServiceDefaultAudienceMode(defaultConfig.AudienceMode),
			"api_url":       strings.TrimSpace(defaultConfig.APIURL),
			"updated_at":    now,
		})
	if result.Error != nil {
		return SandboxServiceDefault{}, result.Error
	}
	if result.RowsAffected == 0 {
		var current sandboxServiceDefaultRecord
		if err := db.First(&current, "id = ?", sandboxServiceDefaultID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return SandboxServiceDefault{}, ErrSandboxServiceDefaultAPIKeyRequired
			}
			return SandboxServiceDefault{}, err
		}
		if strings.TrimSpace(current.APIKeyEncrypted) == "" {
			return SandboxServiceDefault{}, ErrSandboxServiceDefaultAPIKeyRequired
		}
	}
	var record sandboxServiceDefaultRecord
	if err := db.First(&record, "id = ?", sandboxServiceDefaultID).Error; err != nil {
		return SandboxServiceDefault{}, err
	}
	return recordToSandboxServiceDefault(record), nil
}

func (s *DBStore) DeleteSandboxServiceDefaultAPIKey() error {
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	return db.Model(&sandboxServiceDefaultRecord{}).
		Where("id = ?", sandboxServiceDefaultID).
		Updates(map[string]any{
			"api_key_encrypted":  "",
			"api_key_updated_at": nil,
			"updated_at":         time.Now().UTC(),
		}).Error
}

func recordToSandboxServiceDefault(record sandboxServiceDefaultRecord) SandboxServiceDefault {
	defaultConfig := SandboxServiceDefault(record)
	defaultConfig.AudienceMode = normalizeSandboxServiceDefaultAudienceMode(defaultConfig.AudienceMode)
	return defaultConfig
}

func (s *DBStore) ListSandboxServiceDefaultAudiences() ([]SandboxServiceDefaultAudience, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	var records []sandboxServiceDefaultAudienceRecord
	if err := db.Order("LOWER(account_login) ASC, id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	audiences := make([]SandboxServiceDefaultAudience, 0, len(records))
	for _, record := range records {
		audiences = append(audiences, SandboxServiceDefaultAudience(record))
	}
	return audiences, nil
}

func (s *DBStore) UpsertSandboxServiceDefaultAudience(audience SandboxServiceDefaultAudience) (SandboxServiceDefaultAudience, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return SandboxServiceDefaultAudience{}, err
	}
	audience.AccountType = normalizeGitHubAccountType(audience.AccountType)
	if audience.GitHubAccountID <= 0 || audience.AccountType == "" {
		return SandboxServiceDefaultAudience{}, fmt.Errorf("valid github account identity is required")
	}
	now := time.Now().UTC()
	record := sandboxServiceDefaultAudienceRecord{
		GitHubAccountID: audience.GitHubAccountID,
		AccountType:     audience.AccountType,
		AccountLogin:    strings.TrimSpace(audience.AccountLogin),
		AccountName:     strings.TrimSpace(audience.AccountName),
		AccountAvatar:   strings.TrimSpace(audience.AccountAvatar),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_type"}, {Name: "github_account_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"account_login", "account_name", "account_avatar", "updated_at"}),
	}).Create(&record).Error; err != nil {
		return SandboxServiceDefaultAudience{}, err
	}
	if err := db.First(&record, "account_type = ? AND github_account_id = ?", record.AccountType, record.GitHubAccountID).Error; err != nil {
		return SandboxServiceDefaultAudience{}, err
	}
	return SandboxServiceDefaultAudience(record), nil
}

func (s *DBStore) DeleteSandboxServiceDefaultAudience(id int64) error {
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	if id <= 0 {
		return nil
	}
	return db.Delete(&sandboxServiceDefaultAudienceRecord{}, "id = ?", id).Error
}

func (s *DBStore) SandboxServiceDefaultAudienceContains(githubAccountID int64, accountType string) (bool, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return false, err
	}
	accountType = normalizeGitHubAccountType(accountType)
	if githubAccountID <= 0 || accountType == "" {
		return false, nil
	}
	var count int64
	if err := db.Model(&sandboxServiceDefaultAudienceRecord{}).
		Where("github_account_id = ? AND account_type = ?", githubAccountID, accountType).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func normalizeSandboxServiceDefaultAudienceMode(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), SandboxServiceDefaultAudienceModeSelected) {
		return SandboxServiceDefaultAudienceModeSelected
	}
	return SandboxServiceDefaultAudienceModeAll
}
