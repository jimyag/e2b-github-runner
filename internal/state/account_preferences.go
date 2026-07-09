package state

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *DBStore) GetAccountPreference(scopeType string, scopeID int64, namespace, key string) (AccountPreference, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return AccountPreference{}, err
	}
	scopeType = normalizeAccountScopeType(scopeType)
	namespace = normalizeAccountPreferencePart(namespace)
	key = normalizeAccountPreferencePart(key)
	if scopeType == "" || scopeID <= 0 || namespace == "" || key == "" {
		return AccountPreference{}, ErrNotFound
	}
	var record accountPreferenceRecord
	if err := db.First(&record, "scope_type = ? AND scope_id = ? AND namespace = ? AND key = ?", scopeType, scopeID, namespace, key).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return AccountPreference{}, ErrNotFound
		}
		return AccountPreference{}, err
	}
	return recordToAccountPreference(record), nil
}

func (s *DBStore) UpsertAccountPreference(preference AccountPreference) (AccountPreference, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return AccountPreference{}, err
	}
	return upsertAccountPreference(db, preference)
}

func (s *DBStore) UpsertAccountPreferenceAndSecret(preference AccountPreference, secret *AccountSecret) (AccountPreference, *AccountSecret, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return AccountPreference{}, nil, err
	}
	var savedPreference AccountPreference
	var savedSecret *AccountSecret
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		savedPreference, err = upsertAccountPreference(tx, preference)
		if err != nil {
			return err
		}
		if secret == nil {
			return nil
		}
		secretValue, err := upsertAccountSecret(tx, *secret)
		if err != nil {
			return err
		}
		savedSecret = &secretValue
		return nil
	}); err != nil {
		return AccountPreference{}, nil, err
	}
	return savedPreference, savedSecret, nil
}

func (s *DBStore) UpsertAccountPreferenceAndDeleteSecret(preference AccountPreference, secret AccountSecret) (AccountPreference, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return AccountPreference{}, err
	}
	var savedPreference AccountPreference
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		savedPreference, err = upsertAccountPreference(tx, preference)
		if err != nil {
			return err
		}
		if err := deleteAccountSecret(tx, secret.ScopeType, secret.ScopeID, secret.KeyType); err != nil && err != ErrNotFound {
			return err
		}
		return nil
	}); err != nil {
		return AccountPreference{}, err
	}
	return savedPreference, nil
}

func upsertAccountPreference(db *gorm.DB, preference AccountPreference) (AccountPreference, error) {
	scopeType := normalizeAccountScopeType(preference.ScopeType)
	namespace := normalizeAccountPreferencePart(preference.Namespace)
	key := normalizeAccountPreferencePart(preference.Key)
	if scopeType == "" {
		return AccountPreference{}, fmt.Errorf("scope_type is required")
	}
	if preference.ScopeID <= 0 {
		return AccountPreference{}, fmt.Errorf("scope_id is required")
	}
	if namespace == "" {
		return AccountPreference{}, fmt.Errorf("namespace is required")
	}
	if key == "" {
		return AccountPreference{}, fmt.Errorf("key is required")
	}
	valueJSON := strings.TrimSpace(preference.ValueJSON)
	if valueJSON == "" {
		return AccountPreference{}, fmt.Errorf("value_json is required")
	}
	now := time.Now().UTC()
	record := accountPreferenceRecord{
		ScopeType: scopeType,
		ScopeID:   preference.ScopeID,
		Namespace: namespace,
		Key:       key,
		ValueJSON: valueJSON,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "scope_type"}, {Name: "scope_id"}, {Name: "namespace"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value_json", "updated_at"}),
	}).Create(&record).Error; err != nil {
		return AccountPreference{}, err
	}
	if err := db.First(&record, "scope_type = ? AND scope_id = ? AND namespace = ? AND key = ?", scopeType, preference.ScopeID, namespace, key).Error; err != nil {
		return AccountPreference{}, err
	}
	return recordToAccountPreference(record), nil
}

func recordToAccountPreference(record accountPreferenceRecord) AccountPreference {
	return AccountPreference(record)
}

func normalizeAccountPreferencePart(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
