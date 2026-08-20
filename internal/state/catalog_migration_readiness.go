package state

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
)

const (
	maxCatalogMigrationReplayInputs  = 5000
	maxCatalogMigrationReplaySamples = 20
	maxCatalogMigrationChanges       = 100
)

type catalogMigrationReplayInput struct {
	RepositoryFullName  string                    `gorm:"column:repository_full_name"`
	RequestedLabelsJSON string                    `gorm:"column:requested_labels_json"`
	RequestCount        int64                     `gorm:"column:request_count"`
	FirstSeenAt         catalogMigrationTimestamp `gorm:"column:first_seen_at"`
	LastSeenAt          catalogMigrationTimestamp `gorm:"column:last_seen_at"`
}

type catalogMigrationTimestamp struct {
	time.Time
}

func (timestamp *catalogMigrationTimestamp) Scan(value any) error {
	if value == nil {
		timestamp.Time = time.Time{}
		return nil
	}
	parsed := catalogMigrationTime(value)
	if parsed.IsZero() {
		return fmt.Errorf("unsupported catalog migration timestamp %T %v", value, value)
	}
	timestamp.Time = parsed
	return nil
}

func (timestamp catalogMigrationTimestamp) Value() (driver.Value, error) {
	if timestamp.Time.IsZero() {
		return nil, nil
	}
	return timestamp.Time, nil
}

type catalogMigrationLifecycleAggregate struct {
	ProfileName              string `gorm:"column:profile_name"`
	RequestCount             int64  `gorm:"column:request_count"`
	RegisteredRequests       int64  `gorm:"column:registered_requests"`
	CompletedRequests        int64  `gorm:"column:completed_requests"`
	CleanupFinalizedRequests int64  `gorm:"column:cleanup_finalized_requests"`
}

func (s *DBStore) CatalogMigrationReadiness(start, end time.Time) (CatalogMigrationReadiness, error) {
	start = start.UTC()
	end = end.UTC()
	if start.IsZero() || end.IsZero() || !start.Before(end) {
		return CatalogMigrationReadiness{}, fmt.Errorf("catalog migration readiness window must have start before end")
	}
	db, err := s.dbOrEnsure()
	if err != nil {
		return CatalogMigrationReadiness{}, err
	}
	report := CatalogMigrationReadiness{
		WindowStart:   start,
		WindowEnd:     end,
		ReplaySamples: []CatalogMatchReplaySample{},
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		snapshot, err := loadCatalogMatchSnapshot(tx)
		if err != nil {
			return err
		}
		if err := populateCatalogMigrationReplay(tx, snapshot, start, end, &report); err != nil {
			return err
		}
		if err := populateCatalogMigrationLifecycle(tx, snapshot.profiles, start, end, &report); err != nil {
			return err
		}
		return populateCatalogMigrationChanges(tx, start, end, &report)
	}, catalogSnapshotTxOptions)
	if err != nil {
		return CatalogMigrationReadiness{}, err
	}
	return report, nil
}

func populateCatalogMigrationReplay(
	tx *gorm.DB,
	snapshot catalogMatchSnapshot,
	start, end time.Time,
	report *CatalogMigrationReadiness,
) error {
	requestScope := tx.Model(&runnerRequestRecord{}).
		Where("queued_at >= ? AND queued_at < ?", start, end).
		Where("repository_full_name <> '' AND requested_labels_json <> ''")
	if err := requestScope.Count(&report.Replay.RequestCount).Error; err != nil {
		return err
	}
	var inputs []catalogMigrationReplayInput
	if err := requestScope.
		Select(`repository_full_name, requested_labels_json, COUNT(*) AS request_count,
			MIN(queued_at) AS first_seen_at, MAX(queued_at) AS last_seen_at`).
		Group("repository_full_name, requested_labels_json").
		Order("repository_full_name ASC, requested_labels_json ASC").
		Limit(maxCatalogMigrationReplayInputs + 1).
		Scan(&inputs).Error; err != nil {
		return err
	}
	if len(inputs) > maxCatalogMigrationReplayInputs {
		report.Replay.Truncated = true
		inputs = inputs[:maxCatalogMigrationReplayInputs]
	}
	report.Replay.DistinctInputCount = len(inputs)
	for _, input := range inputs {
		var labels []string
		if err := json.Unmarshal([]byte(input.RequestedLabelsJSON), &labels); err != nil {
			report.Replay.ErrorRequests += input.RequestCount
			appendCatalogMigrationReplaySample(report, CatalogMatchReplaySample{
				RepositoryFullName: input.RepositoryFullName,
				RequestCount:       input.RequestCount,
				FirstSeenAt:        input.FirstSeenAt.Time,
				LastSeenAt:         input.LastSeenAt.Time,
				Result:             "error",
				Error:              "decode requested labels: " + err.Error(),
			})
			continue
		}
		comparison := snapshot.compare(input.RepositoryFullName, labels)
		result := comparison.Result()
		switch result {
		case "same":
			report.Replay.SameRequests += input.RequestCount
		case "legacy_only":
			report.Replay.LegacyOnlyRequests += input.RequestCount
		case "enabled_only":
			report.Replay.EnabledOnlyRequests += input.RequestCount
		case "different_profile":
			report.Replay.DifferentProfileRequests += input.RequestCount
		default:
			return fmt.Errorf("unexpected catalog match comparison result %q", result)
		}
		if result != "same" {
			appendCatalogMigrationReplaySample(report, CatalogMatchReplaySample{
				RepositoryFullName: input.RepositoryFullName,
				Labels:             append([]string(nil), labels...),
				RequestCount:       input.RequestCount,
				FirstSeenAt:        input.FirstSeenAt.Time,
				LastSeenAt:         input.LastSeenAt.Time,
				Result:             result,
				LegacyProfile:      catalogMigrationProfileName(comparison.Legacy),
				LegacyReason:       comparison.Legacy.Reason,
				EnabledProfile:     catalogMigrationProfileName(comparison.Enabled),
				EnabledReason:      comparison.Enabled.Reason,
			})
		}
	}
	return nil
}

func catalogMigrationTime(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC()
	case string:
		return parseCatalogMigrationTime(typed)
	case []byte:
		return parseCatalogMigrationTime(string(typed))
	default:
		return time.Time{}
	}
}

func parseCatalogMigrationTime(value string) time.Time {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func appendCatalogMigrationReplaySample(report *CatalogMigrationReadiness, sample CatalogMatchReplaySample) {
	if len(report.ReplaySamples) >= maxCatalogMigrationReplaySamples {
		return
	}
	report.ReplaySamples = append(report.ReplaySamples, sample)
}

func catalogMigrationProfileName(match ProfileMatch) string {
	if match.Profile == nil {
		return ""
	}
	return match.Profile.Name
}

func populateCatalogMigrationLifecycle(
	tx *gorm.DB,
	profiles []RunnerProfile,
	start, end time.Time,
	report *CatalogMigrationReadiness,
) error {
	var aggregates []catalogMigrationLifecycleAggregate
	if err := tx.Model(&runnerRequestRecord{}).
		Select(`profile_name, COUNT(*) AS request_count,
			SUM(CASE WHEN running_at IS NOT NULL AND (assigned_job_id <> 0 OR assigned_job_name <> '') THEN 1 ELSE 0 END) AS registered_requests,
			SUM(CASE WHEN status = ? AND completed_at IS NOT NULL AND (assigned_job_id <> 0 OR assigned_job_name <> '') THEN 1 ELSE 0 END) AS completed_requests,
			SUM(CASE WHEN status = ? AND completed_at IS NOT NULL AND (assigned_job_id <> 0 OR assigned_job_name <> '') THEN 1 ELSE 0 END) AS cleanup_finalized_requests`, StatusCompleted, StatusCompleted).
		Where("queued_at >= ? AND queued_at < ? AND profile_name <> ''", start, end).
		Group("profile_name").
		Scan(&aggregates).Error; err != nil {
		return err
	}
	aggregatesByProfile := make(map[string]catalogMigrationLifecycleAggregate, len(aggregates))
	for _, aggregate := range aggregates {
		aggregatesByProfile[aggregate.ProfileName] = aggregate
	}
	enabledProfiles := make([]RunnerProfile, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Enabled {
			enabledProfiles = append(enabledProfiles, profile)
		}
	}
	sort.Slice(enabledProfiles, func(i, j int) bool { return enabledProfiles[i].Name < enabledProfiles[j].Name })
	report.Specs = make([]RunnerSpecLifecycleEvidence, 0, len(enabledProfiles))
	for _, profile := range enabledProfiles {
		aggregate := aggregatesByProfile[profile.Name]
		evidence := RunnerSpecLifecycleEvidence{
			Name:                     profile.Name,
			WorkflowLabels:           append([]string(nil), profile.RequiredLabels...),
			RequestCount:             aggregate.RequestCount,
			RegisteredRequests:       aggregate.RegisteredRequests,
			CompletedRequests:        aggregate.CompletedRequests,
			CleanupFinalizedRequests: aggregate.CleanupFinalizedRequests,
		}
		if aggregate.CleanupFinalizedRequests > 0 {
			latest, err := latestCatalogMigrationLifecycleExample(tx, profile.Name, start, end)
			if err != nil {
				return err
			}
			evidence.Latest = latest
		}
		report.Specs = append(report.Specs, evidence)
	}
	return nil
}

func latestCatalogMigrationLifecycleExample(
	tx *gorm.DB,
	profileName string,
	start, end time.Time,
) (*RunnerSpecLifecycleExample, error) {
	var record runnerRequestRecord
	err := tx.Select(runnerRequestListSelectColumns).
		Where("queued_at >= ? AND queued_at < ?", start, end).
		Where("profile_name = ? AND status = ? AND completed_at IS NOT NULL", profileName, StatusCompleted).
		Where("assigned_job_id <> 0 OR assigned_job_name <> ''").
		Order("completed_at DESC, id ASC").
		First(&record).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	state := recordToState(record)
	return &RunnerSpecLifecycleExample{
		RequestID:          state.ID,
		RepositoryFullName: state.RepositoryFullName,
		WorkflowJobID:      state.AssignedJobID,
		GitHubJobURL:       state.GitHubJobURL,
		RequestedLabels:    append([]string(nil), state.RequestedLabels...),
		RegisteredAt:       state.RunningAt,
		CompletedAt:        state.CompletedAt,
		CleanupFinalizedAt: state.CompletedAt,
	}, nil
}

func populateCatalogMigrationChanges(
	tx *gorm.DB,
	start, end time.Time,
	report *CatalogMigrationReadiness,
) error {
	var records []auditEventRecord
	if err := tx.
		Where("created_at >= ? AND created_at < ?", start, end).
		Where(`action LIKE ? OR action LIKE ? OR action LIKE ? OR action LIKE ? OR action = ? OR action = ?`,
			"profile.%", "runner_group.%", "repository_policy.%", "sandbox_default.%",
			"sandbox.configure", "sandbox_api_key.delete").
		Order("created_at DESC, id DESC").
		Limit(maxCatalogMigrationChanges + 1).
		Find(&records).Error; err != nil {
		return err
	}
	if len(records) > maxCatalogMigrationChanges {
		report.CatalogChangesTruncated = true
		records = records[:maxCatalogMigrationChanges]
	}
	report.CatalogChanges = make([]AuditEvent, 0, len(records))
	for _, record := range records {
		report.CatalogChanges = append(report.CatalogChanges, auditEventFromRecord(record))
	}
	return nil
}
