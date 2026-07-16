package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetAccount returns a local account by its stable database ID.
func (s *DBStore) GetAccount(accountID int64) (Account, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return Account{}, err
	}
	if accountID <= 0 {
		return Account{}, ErrNotFound
	}
	var record accountRecord
	if err := db.First(&record, "id = ?", accountID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Account{}, ErrNotFound
		}
		return Account{}, err
	}
	return Account(record), nil
}

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

// ListAccounts returns one page of accounts with their linked OAuth identities.
func (s *DBStore) ListAccounts(options AccountListOptions) ([]AccountListItem, int64, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return nil, 0, err
	}
	if options.Limit <= 0 {
		options.Limit = 50
	}
	if options.Offset < 0 {
		return nil, 0, fmt.Errorf("offset must be a non-negative integer")
	}

	query := db.Model(&accountRecord{})
	if role := strings.TrimSpace(options.Role); role != "" {
		normalizedRole := normalizePlatformRole(role)
		if normalizedRole == "" {
			return nil, 0, fmt.Errorf("role must be admin or user")
		}
		query = query.Where("accounts.role = ?", normalizedRole)
	}
	if search := strings.ToLower(strings.TrimSpace(options.Query)); search != "" {
		like := "%" + search + "%"
		query = query.Where(`EXISTS (
			SELECT 1 FROM oauth_identities
			WHERE oauth_identities.account_id = accounts.id
			AND (
				LOWER(oauth_identities.oauth_provider) LIKE ? OR
				LOWER(oauth_identities.oauth_subject) LIKE ? OR
				LOWER(oauth_identities.oauth_login) LIKE ?
			)
		)`, like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []accountRecord
	if err := query.Order("accounts.updated_at DESC").Order("accounts.id DESC").Limit(options.Limit).Offset(options.Offset).Find(&records).Error; err != nil {
		return nil, 0, err
	}

	items := make([]AccountListItem, len(records))
	if len(records) == 0 {
		return items, total, nil
	}
	accountIDs := make([]int64, len(records))
	itemByAccountID := make(map[int64]*AccountListItem, len(records))
	for index, record := range records {
		items[index] = AccountListItem{
			Account:         Account(record),
			OAuthIdentities: []OAuthIdentity{},
		}
		accountIDs[index] = record.ID
		itemByAccountID[record.ID] = &items[index]
	}

	var identities []oauthIdentityRecord
	if err := db.Where("account_id IN ?", accountIDs).Order("oauth_provider ASC").Order("id ASC").Find(&identities).Error; err != nil {
		return nil, 0, err
	}
	for _, identity := range identities {
		item, ok := itemByAccountID[identity.AccountID]
		if !ok || item == nil {
			return nil, 0, fmt.Errorf("oauth identity %d references unexpected account %d", identity.ID, identity.AccountID)
		}
		item.OAuthIdentities = append(
			item.OAuthIdentities,
			recordToOAuthIdentity(identity),
		)
	}
	return items, total, nil
}

// GetAccountStats returns unfiltered account, role, and OAuth identity totals.
func (s *DBStore) GetAccountStats() (AccountStats, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return AccountStats{}, err
	}
	var stats AccountStats
	var roleCounts []struct {
		Role  string
		Count int64
	}
	if err := db.Model(&accountRecord{}).
		Select("role, COUNT(*) AS count").
		Group("role").
		Scan(&roleCounts).Error; err != nil {
		return AccountStats{}, err
	}
	for _, roleCount := range roleCounts {
		stats.TotalAccounts += roleCount.Count
		switch roleCount.Role {
		case "admin":
			stats.AdminAccounts = roleCount.Count
		case "user":
			stats.UserAccounts = roleCount.Count
		}
	}
	if err := db.Model(&oauthIdentityRecord{}).Count(&stats.OAuthIdentities).Error; err != nil {
		return AccountStats{}, err
	}
	return stats, nil
}

// UpdateAccountRoleWithAudit atomically updates an account role and records the administrator action.
func (s *DBStore) UpdateAccountRoleWithAudit(update AccountRoleUpdate) (Account, error) {
	db, err := s.dbOrEnsure()
	if err != nil {
		return Account{}, err
	}
	if update.ActorAccountID <= 0 || update.AccountID <= 0 {
		return Account{}, ErrNotFound
	}
	if update.ActorAccountID == update.AccountID {
		return Account{}, ErrConflict
	}
	role := normalizePlatformRole(update.Role)
	if role == "" {
		return Account{}, fmt.Errorf("role must be admin or user")
	}
	auditActor := strings.TrimSpace(update.AuditActor)
	if auditActor == "" {
		return Account{}, fmt.Errorf("audit actor is required")
	}

	var updated Account
	var lastErr error
	for attempt := 0; attempt < 20; attempt++ {
		updated, lastErr = updateAccountRoleWithAuditOnce(db, update.ActorAccountID, update.AccountID, role, auditActor)
		if lastErr == nil {
			return updated, nil
		}
		if !isTransientStoreError(lastErr) {
			return Account{}, lastErr
		}
		time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
	}
	return Account{}, lastErr
}

func updateAccountRoleWithAuditOnce(db *gorm.DB, actorAccountID, accountID int64, role, auditActor string) (Account, error) {
	var updated accountRecord
	err := db.Transaction(func(tx *gorm.DB) error {
		var actor accountRecord
		if err := tx.First(&actor, "id = ?", actorAccountID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if actor.Role != "admin" {
			return ErrConflict
		}

		var target accountRecord
		if err := tx.First(&target, "id = ?", accountID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if target.ID == actor.ID {
			return ErrConflict
		}
		if target.Role == role {
			updated = target
			return nil
		}
		if target.Role == "admin" && role == "user" {
			// This predicate read is part of the serializable invariant: concurrent
			// administrators must not both demote each other and leave no admin.
			var adminCount int64
			if err := tx.Model(&accountRecord{}).Where("role = ?", "admin").Count(&adminCount).Error; err != nil {
				return err
			}
			if adminCount <= 1 {
				return ErrConflict
			}
		}

		now := time.Now().UTC()
		result := tx.Model(&accountRecord{}).
			Where("id = ? AND role = ?", target.ID, target.Role).
			Updates(map[string]any{"role": role, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}

		payload, err := json.Marshal(map[string]string{
			"old_role": target.Role,
			"new_role": role,
		})
		if err != nil {
			return err
		}
		if _, err := appendAuditEvent(tx, AuditEvent{
			Actor:        auditActor,
			Action:       "account.role.update",
			ResourceType: "account",
			ResourceID:   fmt.Sprint(target.ID),
			PayloadJSON:  string(payload),
			CreatedAt:    now,
		}); err != nil {
			return err
		}

		target.Role = role
		target.UpdatedAt = now
		updated = target
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Account{}, err
	}
	return Account(updated), nil
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
	return appendAuditEvent(db, event)
}

func appendAuditEvent(db *gorm.DB, event AuditEvent) (AuditEvent, error) {
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
