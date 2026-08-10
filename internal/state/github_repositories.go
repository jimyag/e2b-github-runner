package state

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *DBStore) GetGitHubRepository(id int64) (GitHubRepository, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return GitHubRepository{}, err
	}
	if id <= 0 {
		return GitHubRepository{}, ErrNotFound
	}
	var record githubRepositoryRecord
	if err := db.First(&record, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return GitHubRepository{}, ErrNotFound
		}
		return GitHubRepository{}, err
	}
	return GitHubRepository(record), nil
}

func (s *DBStore) GetGitHubRepositoryByName(fullName string) (GitHubRepository, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return GitHubRepository{}, err
	}
	fullName = strings.TrimSpace(fullName)
	if fullName == "" {
		return GitHubRepository{}, ErrNotFound
	}
	var record githubRepositoryRecord
	if err := db.First(&record, "full_name = ?", fullName).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return GitHubRepository{}, ErrNotFound
		}
		return GitHubRepository{}, err
	}
	return GitHubRepository(record), nil
}

func (s *DBStore) UpsertGitHubRepository(repository GitHubRepository) (GitHubRepository, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return GitHubRepository{}, err
	}
	if repository.ID <= 0 || strings.TrimSpace(repository.FullName) == "" || repository.InstallationID <= 0 {
		return GitHubRepository{}, errors.New("github repository id, full name, and installation id are required")
	}
	now := time.Now().UTC()
	record := githubRepositoryRecord{
		ID:             repository.ID,
		FullName:       strings.TrimSpace(repository.FullName),
		InstallationID: repository.InstallationID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"full_name", "installation_id", "updated_at"}),
	}).Create(&record).Error; err != nil {
		return GitHubRepository{}, err
	}
	if err := db.First(&record, "id = ?", repository.ID).Error; err != nil {
		return GitHubRepository{}, err
	}
	return GitHubRepository(record), nil
}
