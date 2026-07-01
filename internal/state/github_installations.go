package state

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *DBStore) ListGitHubInstallations(accountID int64) ([]GitHubInstallation, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	if accountID <= 0 {
		return nil, ErrNotFound
	}
	var records []githubInstallationRecord
	if err := db.
		Where("account_id = ?", accountID).
		Order("account_login ASC, installation_id ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	repositories, err := repositoriesForGitHubInstallations(db, records)
	if err != nil {
		return nil, err
	}
	installations := make([]GitHubInstallation, 0, len(records))
	for _, record := range records {
		installations = append(installations, recordToGitHubInstallation(record, repositories[record.ID]))
	}
	return installations, nil
}

func (s *DBStore) UpsertGitHubInstallation(installation GitHubInstallation) (GitHubInstallation, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return GitHubInstallation{}, err
	}
	if installation.AccountID <= 0 {
		return GitHubInstallation{}, fmt.Errorf("account_id is required")
	}
	if installation.InstallationID <= 0 {
		return GitHubInstallation{}, fmt.Errorf("installation_id is required")
	}
	now := time.Now().UTC()
	record := githubInstallationRecord{
		AccountID:      installation.AccountID,
		InstallationID: installation.InstallationID,
		AccountLogin:   strings.TrimSpace(installation.AccountLogin),
		AccountName:    strings.TrimSpace(installation.AccountName),
		AccountAvatar:  strings.TrimSpace(installation.AccountAvatar),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "account_id"}, {Name: "installation_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"account_login", "account_name", "account_avatar", "updated_at"}),
	}).Create(&record).Error; err != nil {
		return GitHubInstallation{}, err
	}
	if err := db.First(&record, "account_id = ? AND installation_id = ?", installation.AccountID, installation.InstallationID).Error; err != nil {
		return GitHubInstallation{}, err
	}
	return recordToGitHubInstallation(record, nil), nil
}

func (s *DBStore) DeleteGitHubInstallation(accountID, id int64) error {
	db, err := s.dbOrEnsure()
	if err != nil {
		return err
	}
	if accountID <= 0 || id <= 0 {
		return ErrNotFound
	}
	result := db.Where("account_id = ? AND id = ?", accountID, id).Delete(&githubInstallationRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func repositoriesForGitHubInstallations(db *gorm.DB, records []githubInstallationRecord) (map[int64][]string, error) {
	out := make(map[int64][]string, len(records))
	if len(records) == 0 {
		return out, nil
	}
	installationIDs := make([]int64, 0, len(records))
	localIDsByInstallationID := map[int64][]int64{}
	for _, record := range records {
		installationIDs = append(installationIDs, record.InstallationID)
		localIDsByInstallationID[record.InstallationID] = append(localIDsByInstallationID[record.InstallationID], record.ID)
	}
	var rows []struct {
		GitHubInstallationID int64  `gorm:"column:github_installation_id"`
		RepositoryFullName   string `gorm:"column:repository_full_name"`
	}
	if err := db.Model(&runnerRequestRecord{}).
		Select("github_installation_id, repository_full_name").
		Where("github_installation_id IN ?", uniquePositiveInt64s(installationIDs)).
		Where("repository_full_name != ''").
		Group("github_installation_id, repository_full_name").
		Order("repository_full_name ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		for _, localID := range localIDsByInstallationID[row.GitHubInstallationID] {
			out[localID] = append(out[localID], row.RepositoryFullName)
		}
	}
	return out, nil
}

func recordToGitHubInstallation(record githubInstallationRecord, repositories []string) GitHubInstallation {
	return GitHubInstallation{
		ID:             record.ID,
		AccountID:      record.AccountID,
		InstallationID: record.InstallationID,
		AccountLogin:   record.AccountLogin,
		AccountName:    record.AccountName,
		AccountAvatar:  record.AccountAvatar,
		Repositories:   normalizeRepositories(repositories),
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}

func normalizeRepositories(repositories []string) []string {
	out := make([]string, 0, len(repositories))
	seen := map[string]bool{}
	for _, repository := range repositories {
		repository = strings.TrimSpace(repository)
		if repository == "" || strings.Contains(repository, "*") || !strings.Contains(repository, "/") || seen[repository] {
			continue
		}
		seen[repository] = true
		out = append(out, repository)
	}
	return out
}
