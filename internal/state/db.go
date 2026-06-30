package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type DBStore struct {
	opts     Options
	mu       sync.Mutex
	db       *gorm.DB
	migrated bool
}

func New(dir string) Store {
	return NewWithOptions(Options{
		Backend:        BackendSQLite,
		DatabaseDSN:    filepath.Join(dir, "runnerd.db"),
		MigrateOnStart: true,
	})
}

func NewWithOptions(opts Options) Store {
	backend := strings.ToLower(strings.TrimSpace(opts.Backend))
	if backend == "" {
		backend = BackendSQLite
	}
	if opts.DatabaseDSN == "" && backend == BackendSQLite {
		opts.DatabaseDSN = filepath.Join(".", "var", "runnerd.db")
	}
	opts.Backend = backend
	return &DBStore{opts: opts}
}

func (s *DBStore) Ensure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return nil
	}

	db, err := s.open()
	if err != nil {
		return err
	}
	if s.opts.MigrateOnStart && !s.migrated {
		if err := s.migrate(db); err != nil {
			return err
		}
		s.migrated = true
	}
	if s.opts.Backend == BackendSQLite {
		if err := enableSQLiteForeignKeys(db); err != nil {
			return err
		}
	}
	s.db = db
	return nil
}

func (s *DBStore) dbOrEnsure() (*gorm.DB, error) {
	if err := s.Ensure(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db, nil
}

func (s *DBStore) open() (*gorm.DB, error) {
	cfg := &gorm.Config{
		Logger:                                   gormlogger.Default.LogMode(gormlogger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	}
	switch s.opts.Backend {
	case BackendSQLite:
		dir := filepath.Dir(s.opts.DatabaseDSN)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
		db, err := gorm.Open(sqlite.Open(s.opts.DatabaseDSN), cfg)
		if err != nil {
			return nil, err
		}
		if err := configureSQLite(db); err != nil {
			return nil, err
		}
		return db, nil
	case BackendPostgres:
		return gorm.Open(postgres.Open(s.opts.DatabaseDSN), cfg)
	case BackendMySQL:
		dsn, err := mysqlDSNWithParseTime(s.opts.DatabaseDSN)
		if err != nil {
			return nil, err
		}
		return gorm.Open(gormmysql.Open(dsn), cfg)
	default:
		return nil, fmt.Errorf("unsupported state backend: %s", s.opts.Backend)
	}
}

func configureSQLite(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 15000",
	}
	for _, pragma := range pragmas {
		if err := db.Exec(pragma).Error; err != nil {
			return fmt.Errorf("%s: %w", pragma, err)
		}
	}
	return nil
}

func enableSQLiteForeignKeys(db *gorm.DB) error {
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		return fmt.Errorf("PRAGMA foreign_keys = ON: %w", err)
	}
	return nil
}

func mysqlDSNWithParseTime(dsn string) (string, error) {
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return "", err
	}
	cfg.ParseTime = true
	return cfg.FormatDSN(), nil
}

func (s *DBStore) migrate(db *gorm.DB) error {
	if err := migrateLegacySchemaColumns(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(
		&runnerRequestRecord{},
		&runnerEventRecord{},
		&runnerProfileRecord{},
		&runnerGroupRecord{},
		&runnerGroupSpecRecord{},
		&repositoryPolicyRecord{},
		&auditEventRecord{},
		&accountRecord{},
		&oauthIdentityRecord{},
		&githubInstallationRecord{},
	); err != nil {
		return err
	}
	return nil
}

func migrateLegacySchemaColumns(db *gorm.DB) error {
	migrator := db.Migrator()
	if migrator.HasTable(&oauthIdentityRecord{}) {
		for _, constraint := range []string{"Account", "fk_oauth_identities_account", "oauth_identities_account_id_fkey"} {
			if migrator.HasConstraint(&oauthIdentityRecord{}, constraint) {
				if err := migrator.DropConstraint(&oauthIdentityRecord{}, constraint); err != nil {
					return err
				}
			}
		}
	}
	if migrator.HasTable(&runnerProfileRecord{}) && !migrator.HasColumn(&runnerProfileRecord{}, "default_available") {
		if err := migrator.AddColumn(&legacyRunnerProfileDefaultAvailableColumn{}, "DefaultAvailable"); err != nil {
			return err
		}
	}
	if migrator.HasTable(&repositoryPolicyRecord{}) && !migrator.HasColumn(&repositoryPolicyRecord{}, "runner_group_name") {
		if err := migrator.AddColumn(&legacyRepositoryPolicyRunnerGroupNameColumn{}, "RunnerGroupName"); err != nil {
			return err
		}
	}
	return nil
}
