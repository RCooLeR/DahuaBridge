package archive

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	mysql "github.com/go-sql-driver/mysql"
)

func TestArchiveMySQLConfigFromDiscreteSettings(t *testing.T) {
	parsed, err := archiveMySQLConfig(config.ArchiveConfig{
		DBHost: "mysql", DBPort: 3306, DBName: "dahuabridge", DBUser: "bridge", DBPassword: "secret", DBTLS: "skip-verify",
	})
	if err != nil {
		t.Fatalf("archiveMySQLConfig returned error: %v", err)
	}
	if parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "dahuabridge" || parsed.User != "bridge" || parsed.Passwd != "secret" {
		t.Fatalf("unexpected parsed mysql config: network=%q address=%q database=%q user=%q", parsed.Net, parsed.Addr, parsed.DBName, parsed.User)
	}
	if parsed.TLSConfig != "skip-verify" {
		t.Fatalf("unexpected TLS mode %q", parsed.TLSConfig)
	}
}

func TestPrepareArchiveMySQLConfigUsesCompatibleHandshakeCollation(t *testing.T) {
	prepared, err := prepareArchiveMySQLConfig(config.ArchiveConfig{
		DBHost: "mysql", DBPort: 3306, DBName: "dahuabridge", DBUser: "bridge", DBPassword: "secret",
	})
	if err != nil {
		t.Fatalf("prepareArchiveMySQLConfig returned error: %v", err)
	}
	if prepared.Collation != "utf8mb4_bin" {
		t.Fatalf("unexpected default handshake collation %q", prepared.Collation)
	}
	if prepared.TLSConfig != "true" {
		t.Fatalf("unexpected default TLS mode %q", prepared.TLSConfig)
	}
	if _, err := mysql.NewConnector(prepared); err != nil {
		t.Fatalf("default MySQL connector config was rejected: %v", err)
	}
}

func TestArchiveMySQLConfigAppliesTLSModeToDSNWithoutTLSParameter(t *testing.T) {
	parsed, err := archiveMySQLConfig(config.ArchiveConfig{
		DBDSN: "bridge:secret?tls=false@tcp(mysql:3306)/bridge",
		DBTLS: "skip-verify",
	})
	if err != nil {
		t.Fatalf("archiveMySQLConfig returned error: %v", err)
	}
	if parsed.TLSConfig != "skip-verify" {
		t.Fatalf("db_tls was not applied to DSN without tls parameter: %q", parsed.TLSConfig)
	}
	if parsed.Passwd != "secret?tls=false" {
		t.Fatalf("password containing a query-like value was parsed incorrectly")
	}
}

func TestArchiveMySQLConfigUsesParsedDSNTLSAfterQuestionMarkInPassword(t *testing.T) {
	parsed, err := archiveMySQLConfig(config.ArchiveConfig{
		DBDSN: "bridge:secret?tls=false@tcp(mysql:3306)/bridge?tls=skip-verify",
		DBTLS: "skip-verify",
	})
	if err != nil {
		t.Fatalf("archiveMySQLConfig returned error: %v", err)
	}
	if parsed.TLSConfig != "skip-verify" || parsed.Passwd != "secret?tls=false" {
		t.Fatalf("unexpected parsed DSN: tls=%q password=%q", parsed.TLSConfig, parsed.Passwd)
	}
}

func TestArchiveMySQLConfigRejectsConflictingDSNTLS(t *testing.T) {
	_, err := archiveMySQLConfig(config.ArchiveConfig{
		DBDSN: "bridge:secret@tcp(mysql:3306)/bridge?tls=false",
		DBTLS: "true",
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected conflicting TLS settings error, got %v", err)
	}
}

func TestArchiveMySQLConfigAcceptsExplicitPlaintextOnlyWhenConfigured(t *testing.T) {
	parsed, err := archiveMySQLConfig(config.ArchiveConfig{
		DBDSN: "bridge:secret@tcp(mysql:3306)/bridge?tls=false",
		DBTLS: "false",
	})
	if err != nil {
		t.Fatalf("archiveMySQLConfig returned error: %v", err)
	}
	if parsed.TLSConfig != "" {
		t.Fatalf("explicit tls=false unexpectedly produced TLS mode %q", parsed.TLSConfig)
	}
}

func TestArchiveMySQLConfigRejectsPreferredTLS(t *testing.T) {
	for name, cfg := range map[string]config.ArchiveConfig{
		"setting": {DBHost: "mysql", DBName: "bridge", DBUser: "bridge", DBPassword: "secret", DBTLS: "preferred"},
		"dsn":     {DBDSN: "bridge:secret@tcp(mysql:3306)/bridge?tls=preferred", DBTLS: "true"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := archiveMySQLConfig(cfg); err == nil || !strings.Contains(err.Error(), "plaintext fallback") {
				t.Fatalf("expected preferred TLS rejection, got %v", err)
			}
		})
	}
}

func TestMySQLConnectRetryClassification(t *testing.T) {
	if !isTransientMySQLConnectError(&mysql.MySQLError{Number: 1040, Message: "too many connections"}) {
		t.Fatal("expected connection-capacity error to be retryable")
	}
	if isTransientMySQLConnectError(&mysql.MySQLError{Number: 1045, Message: "access denied"}) {
		t.Fatal("authentication error must fail without retrying")
	}
	if !isTransientMySQLConnectError(&net.DNSError{Err: "temporary lookup failure", Name: "mysql", IsTemporary: true}) {
		t.Fatal("expected network discovery error to be retryable")
	}
}

func TestOpenArchiveMySQLRetryPreservesContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	db, err := openArchiveMySQLDB(ctx, config.ArchiveConfig{
		DBHost: "127.0.0.1", DBPort: 1, DBName: "bridge", DBUser: "bridge", DBPassword: "secret", DBTLS: "false",
		DBConnectTimeout: time.Minute,
	})
	if db != nil {
		_ = db.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("openArchiveMySQLDB error = %v, want context deadline", err)
	}
}

func TestMySQLUpsertsUseCurrentRowAliasSyntax(t *testing.T) {
	store := &sqlStore{dialect: archiveDialectMySQL}
	queries := []string{
		store.upsertArchiveFilesSQL(),
		store.upsertArchiveEventsSQL(),
		store.upsertClipAssetSQL(),
	}
	for _, query := range queries {
		if !strings.Contains(query, " AS new") || !strings.Contains(query, "ON DUPLICATE KEY UPDATE") {
			t.Fatalf("mysql upsert does not use row aliases: %s", query)
		}
		if strings.Contains(strings.ToUpper(query), "VALUES(") {
			t.Fatalf("mysql upsert uses deprecated VALUES() references: %s", query)
		}
	}
}

func TestMySQLMigrationUsesInsertOnly(t *testing.T) {
	query := mysqlMigrationInsert(archiveMigrationTables[0])
	if strings.Contains(strings.ToUpper(query), "DUPLICATE") || strings.Contains(strings.ToUpper(query), "IGNORE") {
		t.Fatalf("one-time migration must not silently merge target rows: %s", query)
	}
}

func TestArchiveMySQLConfigRejectsUnknownTLSMode(t *testing.T) {
	for _, mode := range []string{"maybe", "off", "disabled"} {
		_, err := archiveMySQLConfig(config.ArchiveConfig{DBHost: "mysql", DBPort: 3306, DBName: "bridge", DBUser: "bridge", DBPassword: "secret", DBTLS: mode})
		if err == nil || !strings.Contains(err.Error(), "db_tls") {
			t.Fatalf("expected db_tls error for %q, got %v", mode, err)
		}
	}
}

func TestOpenArchiveMySQLSecurityOverridesStayDisabled(t *testing.T) {
	parsed, err := archiveMySQLConfig(config.ArchiveConfig{DBDSN: "bridge:secret@tcp(mysql:3306)/bridge?allowAllFiles=true&allowCleartextPasswords=true&allowFallbackToPlaintext=true&allowOldPasswords=true&multiStatements=true"})
	if err != nil {
		t.Fatalf("parse test DSN: %v", err)
	}
	hardenArchiveMySQLConfig(parsed)
	if parsed.MultiStatements || parsed.AllowAllFiles || parsed.AllowCleartextPasswords || parsed.AllowFallbackToPlaintext || parsed.AllowOldPasswords {
		t.Fatal("unsafe MySQL driver options remained enabled")
	}
}
