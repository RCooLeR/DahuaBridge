package archive

import (
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestArchiveSQLiteReadOnlyDSNUsesImmutableOnlyWithoutSidecars(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "archive.db")
	if err := os.WriteFile(sourcePath, []byte("sqlite fixture"), 0o600); err != nil {
		t.Fatalf("write sqlite fixture: %v", err)
	}

	assertImmutable := func(want bool) {
		t.Helper()
		dsn, err := archiveSQLiteReadOnlyDSN(sourcePath)
		if err != nil {
			t.Fatalf("build sqlite read-only DSN: %v", err)
		}
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("parse sqlite read-only DSN: %v", err)
		}
		query := parsed.Query()
		if got := query.Get("mode"); got != "ro" {
			t.Fatalf("mode = %q, want ro", got)
		}
		if got := query.Get("_query_only"); got != "1" {
			t.Fatalf("_query_only = %q, want 1", got)
		}
		if got := query.Get("immutable") == "1"; got != want {
			t.Fatalf("immutable enabled = %t, want %t (DSN %q)", got, want, dsn)
		}
	}

	assertImmutable(true)
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		sidecarPath := sourcePath + suffix
		if err := os.WriteFile(sidecarPath, []byte("sidecar"), 0o600); err != nil {
			t.Fatalf("write %s sidecar: %v", suffix, err)
		}
		assertImmutable(false)
		if err := os.Remove(sidecarPath); err != nil {
			t.Fatalf("remove %s sidecar: %v", suffix, err)
		}
	}
}

func TestOpenArchiveSQLiteReadOnlyAcceptsCleanWALDatabase(t *testing.T) {
	directory := t.TempDir()
	sourcePath := filepath.Join(directory, "archive.db")
	writable, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatalf("open sqlite fixture: %v", err)
	}
	var journalMode string
	if err := writable.QueryRowContext(t.Context(), `PRAGMA journal_mode=WAL`).Scan(&journalMode); err != nil {
		_ = writable.Close()
		t.Fatalf("enable WAL mode: %v", err)
	}
	if journalMode != "wal" {
		_ = writable.Close()
		t.Fatalf("journal mode = %q, want wal", journalMode)
	}
	if _, err := writable.ExecContext(t.Context(), `CREATE TABLE fixture (value TEXT NOT NULL)`); err != nil {
		_ = writable.Close()
		t.Fatalf("create sqlite fixture table: %v", err)
	}
	if _, err := writable.ExecContext(t.Context(), `INSERT INTO fixture(value) VALUES ('committed')`); err != nil {
		_ = writable.Close()
		t.Fatalf("seed sqlite fixture: %v", err)
	}
	if _, err := writable.ExecContext(t.Context(), `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		_ = writable.Close()
		t.Fatalf("checkpoint sqlite fixture: %v", err)
	}
	if err := writable.Close(); err != nil {
		t.Fatalf("close sqlite fixture: %v", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(sourcePath + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("cleanly closed fixture left %s sidecar: %v", suffix, err)
		}
	}
	header, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read sqlite fixture header: %v", err)
	}
	if len(header) < 20 {
		t.Fatalf("sqlite fixture header is only %d bytes", len(header))
	}
	if header[18] != 2 || header[19] != 2 {
		t.Fatalf("sqlite fixture does not retain WAL header bytes: %v", header[18:20])
	}

	// A clean WAL database should remain readable when its containing mount is
	// read-only. Unix permissions model the production read-only bind mount;
	// Windows still exercises immutable/query-only SQL semantics below.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(sourcePath, 0o400); err != nil {
			t.Fatalf("make sqlite fixture read-only: %v", err)
		}
		if err := os.Chmod(directory, 0o500); err != nil {
			t.Fatalf("make sqlite directory read-only: %v", err)
		}
		defer func() {
			_ = os.Chmod(directory, 0o700)
			_ = os.Chmod(sourcePath, 0o600)
		}()
	}

	readonly, err := openArchiveSQLiteReadOnly(sourcePath)
	if err != nil {
		t.Fatalf("open clean WAL database read-only: %v", err)
	}
	defer readonly.Close()
	if err := configureArchiveSQLiteDB(t.Context(), readonly); err != nil {
		t.Fatalf("validate clean WAL database read-only: %v", err)
	}
	var value string
	if err := readonly.QueryRowContext(t.Context(), `SELECT value FROM fixture`).Scan(&value); err != nil {
		t.Fatalf("read clean WAL database: %v", err)
	}
	if value != "committed" {
		t.Fatalf("fixture value = %q, want committed", value)
	}
	var queryOnly int
	if err := readonly.QueryRowContext(t.Context(), `PRAGMA query_only`).Scan(&queryOnly); err != nil {
		t.Fatalf("read query_only pragma: %v", err)
	}
	if queryOnly != 1 {
		t.Fatalf("query_only = %d, want 1", queryOnly)
	}
	if _, err := readonly.ExecContext(t.Context(), `INSERT INTO fixture(value) VALUES ('forbidden')`); err == nil {
		t.Fatal("read-only migration source unexpectedly accepted a write")
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(sourcePath + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read-only open created %s sidecar: %v", suffix, err)
		}
	}
}
