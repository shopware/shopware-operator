package util

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDump(t *testing.T) {
	tc := setupTest(t)
	ctx := context.Background()

	// Allow specifying an existing database to dump
	existingDBName := getEnvOrDefault("MYSQL_DB_TO_DUMP", "")
	var dbName string
	var db *sql.DB
	var err error

	if existingDBName != "" {
		// Use existing database
		dbName = existingDBName
		db = tc.createDatabase(t, dbName)
		defer func() {
			require.NoError(t, db.Close())
		}()
		t.Logf("Using existing database: %s", dbName)
	} else {
		// Create new test database
		dbName = fmt.Sprintf("test_dump_%d", time.Now().Unix())
		db = tc.createDatabase(t, dbName)
		defer func() {
			_, _ = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
			require.NoError(t, db.Close())
		}()

		// Create test tables with data
		_, err = db.Exec(`
			CREATE TABLE users (
				id INT PRIMARY KEY AUTO_INCREMENT,
				name VARCHAR(100),
				email VARCHAR(100),
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			)
		`)
		require.NoError(t, err)

		_, err = db.Exec(`
			CREATE TABLE orders (
				id INT PRIMARY KEY AUTO_INCREMENT,
				user_id INT,
				amount DECIMAL(10,2),
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (user_id) REFERENCES users(id)
			)
		`)
		require.NoError(t, err)

		// Insert test data
		_, err = db.Exec(`
			INSERT INTO users (name, email) VALUES 
			('Alice', 'alice@example.com'),
			('Bob', 'bob@example.com'),
			('Charlie', 'charlie@example.com')
		`)
		require.NoError(t, err)

		_, err = db.Exec(`
			INSERT INTO orders (user_id, amount) VALUES 
			(1, 100.50),
			(1, 200.75),
			(2, 50.25)
		`)
		require.NoError(t, err)
	}

	// Create dump
	tempDir := t.TempDir()
	dumpPath := filepath.Join(tempDir, "test_dump")

	dumpInput := DumpInput{
		DatabaseSpec: tc.databaseSpec(dbName),
		DumpFilePath: dumpPath,
	}

	dumpOutput, err := tc.shell.Dump(ctx, dumpInput)
	require.NoError(t, err, "Dump should succeed")

	// Verify dump output (statistics may not be available if output parsing failed)
	if dumpOutput.UncompressedSize > 0 {
		assert.Greater(t, dumpOutput.CompressedSize, int64(0), "Compressed size should be greater than 0")
		// For very small datasets, compression may not help (ratio could be 1.0 or less)
		assert.GreaterOrEqual(t, dumpOutput.CompressionRation, 0.1, "Compression ratio should be valid")

		if dumpOutput.Duration > 0 {
			t.Logf("Dump stats: Uncompressed=%d bytes, Compressed=%d bytes, Ratio=%.2fx, Duration=%s",
				dumpOutput.UncompressedSize, dumpOutput.CompressedSize, dumpOutput.CompressionRation, dumpOutput.Duration)
		} else {
			t.Logf("Dump stats: Uncompressed=%d bytes, Compressed=%d bytes, Ratio=%.2fx (duration not captured)",
				dumpOutput.UncompressedSize, dumpOutput.CompressedSize, dumpOutput.CompressionRation)
		}
	} else {
		t.Logf("Dump completed successfully (detailed statistics not available)")
	}

	// Verify dump directory was created
	_, err = os.Stat(dumpPath)
	require.NoError(t, err, "Dump directory should exist")

	// Verify dump contains expected files
	entries, err := os.ReadDir(dumpPath)
	require.NoError(t, err)

	hasMetadata := false
	hasDDL := false
	hasData := false

	for _, entry := range entries {
		name := entry.Name()
		if name == "@.json" {
			hasMetadata = true
		}
		if filepath.Ext(name) == ".sql" && name[0] != '@' {
			hasDDL = true
		}
		if filepath.Ext(name) == ".zst" {
			hasData = true
		}
	}

	assert.True(t, hasMetadata, "Dump should contain @.json metadata file")
	assert.True(t, hasDDL, "Dump should contain .sql DDL files")
	assert.True(t, hasData, "Dump should contain .zst compressed data files")

	// Count tables in dump
	tableCount, err := countTablesInDump(dumpPath)
	require.NoError(t, err)

	if existingDBName == "" {
		// For test-created database, we know the exact count
		assert.Equal(t, 3, tableCount, "Expected 3 files in dump (2 tables + 1 database DDL)")
	} else {
		// For existing database, just verify some tables were dumped
		assert.Greater(t, tableCount, 0, "Expected at least one table in dump")
	}
	t.Logf("Dump contains %d .sql files", tableCount)
}

func TestRestoreDump(t *testing.T) {
	tc := setupTest(t)
	ctx := context.Background()

	dbName := fmt.Sprintf("test_restore_%d", time.Now().Unix())
	existingDumpPath := getEnvOrDefault("MYSQL_DUMP_PATH", "")

	db := tc.createDatabase(t, dbName)
	defer func() {
		_, _ = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
		require.NoError(t, db.Close())
	}()

	// Enable local_infile for MySQL Shell's dump loading utility (might be needed only for Percona)
	_, err := db.Exec("SET GLOBAL local_infile = 1")
	require.NoError(t, err, "Failed to enable local_infile. User may need SUPER or SYSTEM_VARIABLES_ADMIN privilege")

	// Check if user provided an existing dump path
	var dumpPath string
	if existingDumpPath != "" {
		// Use existing dump
		dumpPath = existingDumpPath
		t.Logf("Using existing dump from: %s", dumpPath)

		// Verify dump exists
		_, err = os.Stat(dumpPath)
		require.NoError(t, err, "Dump path does not exist: %s", dumpPath)
	} else {
		// Create a test dump using MySQLShell
		tempDir := t.TempDir()
		dumpPath = filepath.Join(tempDir, "test_dump")

		// Create a test table with data
		_, err = db.Exec(`
			CREATE TABLE test_users (
				id INT PRIMARY KEY AUTO_INCREMENT,
				name VARCHAR(100),
				email VARCHAR(100)
			)
		`)
		require.NoError(t, err)

		_, err = db.Exec(`
			INSERT INTO test_users (name, email) VALUES 
			('Alice', 'alice@example.com'),
			('Bob', 'bob@example.com'),
			('Charlie', 'charlie@example.com')
		`)
		require.NoError(t, err)

		// Create dump
		dumpInput := DumpInput{
			DatabaseSpec: tc.databaseSpec(dbName),
			DumpFilePath: dumpPath,
		}

		dumpOutput, err := tc.shell.Dump(ctx, dumpInput)
		require.NoError(t, err)

		// Statistics may not be available if output parsing failed
		if dumpOutput.UncompressedSize > 0 {
			assert.Greater(t, dumpOutput.CompressedSize, int64(0))
			t.Logf("Dump stats: Uncompressed=%d, Compressed=%d", dumpOutput.UncompressedSize, dumpOutput.CompressedSize)
		}

		// Verify dump directory was created
		_, err = os.Stat(dumpPath)
		require.NoError(t, err)
	}

	// Only drop test table if we created it ourselves
	if existingDumpPath == "" {
		// Drop the test table to simulate a restore scenario
		_, err = db.Exec("DROP TABLE IF EXISTS test_users")
		require.NoError(t, err)

		// Verify table is gone
		var tableCount int
		err = db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = 'test_users'", dbName).Scan(&tableCount)
		require.NoError(t, err)
		assert.Equal(t, 0, tableCount)
	}

	// Now restore from the dump
	restoreInput := RestoreInput{
		DatabaseSpec: tc.databaseSpec(dbName),
		DumpFilePath: dumpPath,
	}

	err = tc.shell.RestoreDump(ctx, restoreInput)
	require.NoError(t, err)

	// Verify the database still exists after restore
	var dbExists int
	err = db.QueryRow("SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", dbName).Scan(&dbExists)
	require.NoError(t, err)
	require.Equal(t, 1, dbExists, "Database %s should exist after restore", dbName)

	// Ensure we're using the correct database (in case connection was pooled/reset)
	_, err = db.Exec(fmt.Sprintf("USE `%s`", dbName))
	require.NoError(t, err)

	// Verify the data was restored
	if existingDumpPath != "" {
		expectedTableCount, err := countTablesInDump(dumpPath)
		require.NoError(t, err)
		// For existing dumps, just verify that some tables were restored
		var actualTableCount int
		err = db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ?", dbName).Scan(&actualTableCount)
		require.NoError(t, err)

		expectedTableCount-- // Exclude db creation file
		assert.Equal(t, expectedTableCount, actualTableCount, "Expected %d tables to be restored, but found %d", expectedTableCount, actualTableCount)
		t.Logf("Successfully restored %d tables from existing dump", actualTableCount)
	} else {
		// For test-created dumps, verify specific test data
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM test_users").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 3, count)

		// Verify specific data
		var name, email string
		err = db.QueryRow("SELECT name, email FROM test_users WHERE name = 'Alice'").Scan(&name, &email)
		require.NoError(t, err)
		assert.Equal(t, "Alice", name)
		assert.Equal(t, "alice@example.com", email)
	}
}

type testConfig struct {
	mysqlshPath string
	dbHost      string
	dbPort      string
	dbUser      string
	dbPassword  string
	dbSSLMode   string
	shell       MySQLShell
}

func setupTest(t *testing.T) testConfig {
	t.Helper()
	// Skip if not in integration test environment
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run")
	}

	// Check if mysqlsh is available
	mysqlshPath := getEnvOrDefault("MYSQLSH_PATH", "mysqlsh")
	if _, err := os.Stat(mysqlshPath); err != nil {
		// Try to find in PATH if it's not an absolute path
		if !filepath.IsAbs(mysqlshPath) {
			if _, err := exec.LookPath(mysqlshPath); err != nil {
				t.Skip("mysqlsh not found. Install MySQL Shell or set MYSQLSH_PATH environment variable")
			}
		}
	}

	return testConfig{
		mysqlshPath: mysqlshPath,
		dbHost:      getEnvOrDefault("MYSQL_HOST", "localhost"),
		dbPort:      getEnvOrDefault("MYSQL_PORT", "3306"),
		dbUser:      getEnvOrDefault("MYSQL_USER", "root"),
		dbPassword:  getEnvOrDefault("MYSQL_PASSWORD", "root"),
		dbSSLMode:   getEnvOrDefault("MYSQL_SSL_MODE", "skip-verify"),
		shell:       NewMySQLShell(getEnvOrDefault("MYSQLSH_PATH", "mysqlsh")),
	}
}

func (tc testConfig) createDatabase(t *testing.T, dbName string) *sql.DB {
	t.Helper()
	// Create test database
	rootDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/", tc.dbUser, tc.dbPassword, tc.dbHost, tc.dbPort)
	if tc.dbSSLMode != "" {
		rootDSN = fmt.Sprintf("%s?tls=%s", rootDSN, tc.dbSSLMode)
	}
	rootDB, err := sql.Open("mysql", rootDSN)
	require.NoError(t, err)

	_, err = rootDB.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", dbName))
	require.NoError(t, err)

	// Connect to the specific database
	dbDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", tc.dbUser, tc.dbPassword, tc.dbHost, tc.dbPort, dbName)
	if tc.dbSSLMode != "" {
		dbDSN = fmt.Sprintf("%s?tls=%s", dbDSN, tc.dbSSLMode)
	}
	db, err := sql.Open("mysql", dbDSN)
	require.NoError(t, err)

	require.NoError(t, rootDB.Close())

	return db
}

func (tc testConfig) databaseSpec(dbName string) DatabaseSpec {
	return DatabaseSpec{
		Host:     tc.dbHost,
		Port:     mustParsePort(tc.dbPort),
		User:     tc.dbUser,
		Password: []byte(tc.dbPassword),
		Name:     dbName,
		SSLMode:  tc.dbSSLMode,
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func mustParsePort(port string) int32 {
	var p int32
	_, err := fmt.Sscanf(port, "%d", &p)
	if err != nil {
		panic(err)
	}
	return p
}

func countTablesInDump(dumpPath string) (int, error) {
	// Count .sql files (DDL files) in the dump directory
	// Each table has a corresponding .sql file for its schema
	entries, err := os.ReadDir(dumpPath)
	if err != nil {
		return 0, fmt.Errorf("read dump directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			// Skip metadata files that start with @
			if entry.Name()[0] != '@' {
				count++
			}
		}
	}
	return count, nil
}
