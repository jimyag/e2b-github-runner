package state

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *DBStore) GetAccountByOAuthIdentity(provider, subject string) (Account, OAuthIdentity, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return Account{}, OAuthIdentity{}, err
	}
	provider = normalizeOAuthProvider(provider)
	subject = normalizeOAuthSubject(subject)
	if provider == "" || subject == "" {
		return Account{}, OAuthIdentity{}, ErrNotFound
	}
	var identity oauthIdentityRecord
	if err := db.First(&identity, "oauth_provider = ? AND oauth_subject = ?", provider, subject).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Account{}, OAuthIdentity{}, ErrNotFound
		}
		return Account{}, OAuthIdentity{}, err
	}
	return s.accountFromIdentity(db, identity)
}

func (s *DBStore) GetOAuthIdentityForAccount(accountID int64, provider string) (OAuthIdentity, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return OAuthIdentity{}, err
	}
	provider = normalizeOAuthProvider(provider)
	if accountID <= 0 || provider == "" {
		return OAuthIdentity{}, ErrNotFound
	}
	var identity oauthIdentityRecord
	if err := db.First(&identity, "account_id = ? AND oauth_provider = ?", accountID, provider).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return OAuthIdentity{}, ErrNotFound
		}
		return OAuthIdentity{}, err
	}
	return recordToOAuthIdentity(identity), nil
}

func (s *DBStore) UpsertAccountForOAuthIdentity(identity OAuthIdentity, role string) (Account, OAuthIdentity, error) {
	return s.saveOAuthIdentity(identity, role, true, 0)
}

func (s *DBStore) EnsureAccountForOAuthIdentity(identity OAuthIdentity, role string) (Account, OAuthIdentity, error) {
	return s.saveOAuthIdentity(identity, role, false, 0)
}

func (s *DBStore) LinkOAuthIdentityToAccount(accountID int64, identity OAuthIdentity) (Account, OAuthIdentity, error) {
	return s.saveOAuthIdentity(identity, "", false, accountID)
}

func (s *DBStore) saveOAuthIdentity(identity OAuthIdentity, role string, updateExisting bool, accountID int64) (Account, OAuthIdentity, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return Account{}, OAuthIdentity{}, err
	}
	provider := normalizeOAuthProvider(identity.OAuthProvider)
	subject := normalizeOAuthSubject(identity.OAuthSubject)
	login := normalizeOAuthLogin(identity.OAuthLogin)
	if provider == "" {
		return Account{}, OAuthIdentity{}, fmt.Errorf("oauth_provider is required")
	}
	if subject == "" {
		return Account{}, OAuthIdentity{}, fmt.Errorf("oauth_subject is required")
	}
	if login == "" {
		return Account{}, OAuthIdentity{}, fmt.Errorf("oauth_login is required")
	}
	role = normalizePlatformRole(role)
	if accountID == 0 && role == "" {
		return Account{}, OAuthIdentity{}, fmt.Errorf("role must be admin or user")
	}
	var savedAccount Account
	var savedIdentity OAuthIdentity
	var lastErr error
	for attempt := 0; attempt < 20; attempt++ {
		savedAccount, savedIdentity, lastErr = s.saveOAuthIdentityOnce(db, provider, subject, login, role, identity.CreatedAt, updateExisting, accountID)
		if lastErr == nil {
			return savedAccount, savedIdentity, nil
		}
		if !isTransientStoreError(lastErr) {
			return Account{}, OAuthIdentity{}, lastErr
		}
		time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
	}
	return Account{}, OAuthIdentity{}, lastErr
}

func (s *DBStore) saveOAuthIdentityOnce(db *gorm.DB, provider, subject, login, role string, createdAt time.Time, updateExisting bool, accountID int64) (Account, OAuthIdentity, error) {
	now := time.Now().UTC()
	var savedAccount Account
	var savedIdentity OAuthIdentity
	err := db.Transaction(func(tx *gorm.DB) error {
		var identity oauthIdentityRecord
		err := tx.First(&identity, "oauth_provider = ? AND oauth_subject = ?", provider, subject).Error
		if err == nil {
			if accountID != 0 && identity.AccountID != accountID {
				return ErrConflict
			}
			updates := map[string]any{
				"oauth_login": login,
				"updated_at":  now,
			}
			if err := tx.Model(&identity).Updates(updates).Error; err != nil {
				return err
			}
			if updateExisting {
				if err := tx.Model(&accountRecord{}).Where("id = ?", identity.AccountID).Updates(map[string]any{
					"role":       role,
					"updated_at": now,
				}).Error; err != nil {
					return err
				}
			}
			identity.OAuthLogin = login
			identity.UpdatedAt = now
			var identityErr error
			savedAccount, savedIdentity, identityErr = s.accountFromIdentity(tx, identity)
			return identityErr
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		targetAccountID := accountID
		createdAccountID := int64(0)
		if targetAccountID == 0 {
			account := accountRecord{
				Role:      role,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := tx.Create(&account).Error; err != nil {
				return err
			}
			targetAccountID = account.ID
			createdAccountID = account.ID
		} else {
			var account accountRecord
			if err := tx.First(&account, "id = ?", targetAccountID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrNotFound
				}
				return err
			}
		}
		identity = oauthIdentityRecord{
			AccountID:     targetAccountID,
			OAuthProvider: provider,
			OAuthSubject:  subject,
			OAuthLogin:    login,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if !createdAt.IsZero() {
			identity.CreatedAt = createdAt
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&identity)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if createdAccountID != 0 {
				if err := tx.Delete(&accountRecord{}, "id = ?", createdAccountID).Error; err != nil {
					return err
				}
			}
			if err := tx.First(&identity, "oauth_provider = ? AND oauth_subject = ?", provider, subject).Error; err != nil {
				return err
			}
			if accountID != 0 && identity.AccountID != accountID {
				return ErrConflict
			}
			if err := tx.Model(&identity).Updates(map[string]any{
				"oauth_login": login,
				"updated_at":  now,
			}).Error; err != nil {
				return err
			}
			if updateExisting {
				if err := tx.Model(&accountRecord{}).Where("id = ?", identity.AccountID).Updates(map[string]any{
					"role":       role,
					"updated_at": now,
				}).Error; err != nil {
					return err
				}
			}
			identity.OAuthLogin = login
			identity.UpdatedAt = now
		}
		var identityErr error
		savedAccount, savedIdentity, identityErr = s.accountFromIdentity(tx, identity)
		return identityErr
	})
	if err != nil {
		return Account{}, OAuthIdentity{}, err
	}
	return savedAccount, savedIdentity, nil
}

func isTransientStoreError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "40001") ||
		strings.Contains(message, "40p01") ||
		strings.Contains(message, "sqlstate 40001") ||
		strings.Contains(message, "sqlstate 40p01") ||
		strings.Contains(message, "serialization_failure") ||
		strings.Contains(message, "deadlock") ||
		strings.Contains(message, "deadlock_detected") ||
		strings.Contains(message, "concurrent update") ||
		strings.Contains(message, "lock wait timeout")
}

func (s *DBStore) AppendAuditEvent(event AuditEvent) (AuditEvent, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return AuditEvent{}, err
	}
	record := auditEventRecord{
		Actor:        strings.TrimSpace(event.Actor),
		Action:       strings.TrimSpace(event.Action),
		ResourceType: strings.TrimSpace(event.ResourceType),
		ResourceID:   strings.TrimSpace(event.ResourceID),
		PayloadJSON:  event.PayloadJSON,
		CreatedAt:    event.CreatedAt.UTC(),
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if err := db.Create(&record).Error; err != nil {
		return AuditEvent{}, err
	}
	return auditEventFromRecord(record), nil
}

func (s *DBStore) ListAuditEvents(limit int) ([]AuditEvent, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	var records []auditEventRecord
	if err := db.Order("created_at DESC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	events := make([]AuditEvent, 0, len(records))
	for _, record := range records {
		events = append(events, auditEventFromRecord(record))
	}
	return events, nil
}

func auditEventFromRecord(record auditEventRecord) AuditEvent {
	//lint:ignore S1016 keep record/API mapping explicit so field changes are reviewed intentionally
	return AuditEvent{
		ID:           record.ID,
		Actor:        record.Actor,
		Action:       record.Action,
		ResourceType: record.ResourceType,
		ResourceID:   record.ResourceID,
		PayloadJSON:  record.PayloadJSON,
		CreatedAt:    record.CreatedAt,
	}
}

func (s *DBStore) accountFromIdentity(db *gorm.DB, identity oauthIdentityRecord) (Account, OAuthIdentity, error) {
	var account accountRecord
	if err := db.First(&account, "id = ?", identity.AccountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Account{}, OAuthIdentity{}, ErrNotFound
		}
		return Account{}, OAuthIdentity{}, err
	}
	return Account(account), recordToOAuthIdentity(identity), nil
}

func recordToOAuthIdentity(record oauthIdentityRecord) OAuthIdentity {
	return OAuthIdentity{
		ID:            record.ID,
		AccountID:     record.AccountID,
		OAuthProvider: record.OAuthProvider,
		OAuthSubject:  record.OAuthSubject,
		OAuthLogin:    record.OAuthLogin,
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
	}
}
