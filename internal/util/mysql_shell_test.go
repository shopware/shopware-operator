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

func TestRestoreDump(t *testing.T) {
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

	ctx := context.Background()

	// Setup test database connection
	dbHost := getEnvOrDefault("MYSQL_HOST", "localhost")
	dbPort := getEnvOrDefault("MYSQL_PORT", "3306")
	dbUser := getEnvOrDefault("MYSQL_USER", "root")
	dbPassword := getEnvOrDefault("MYSQL_PASSWORD", "root")
	dbSSLMode := getEnvOrDefault("MYSQL_SSL_MODE", "skip-verify")
	dbName := fmt.Sprintf("test_restore_%d", time.Now().Unix())
	existingDumpPath := getEnvOrDefault("MYSQL_DUMP_PATH", "../../test/data/snapshot/database")

	// Create test database
	rootDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/", dbUser, dbPassword, dbHost, dbPort)
	if dbSSLMode != "" {
		rootDSN = fmt.Sprintf("%s?tls=%s", rootDSN, dbSSLMode)
	}
	db, err := sql.Open("mysql", rootDSN)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", dbName))
	require.NoError(t, err)

	// Close and reconnect with the specific database
	db.Close()
	dbDSN := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", dbUser, dbPassword, dbHost, dbPort, dbName)
	if dbSSLMode != "" {
		dbDSN = fmt.Sprintf("%s?tls=%s", dbDSN, dbSSLMode)
	}
	db, err = sql.Open("mysql", dbDSN)
	require.NoError(t, err)
	defer db.Close()
	defer func() {
		_, _ = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
	}()

	// Enable local_infile for MySQL Shell's dump loading utility (might be needed only for Percona)
	_, err = db.Exec("SET GLOBAL local_infile = 1")
	require.NoError(t, err, "Failed to enable local_infile. User may need SUPER or SYSTEM_VARIABLES_ADMIN privilege")

	shell := NewMySQLShell(mysqlshPath)

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
			DatabaseSpec: DatabaseSpec{
				Host:     dbHost,
				Port:     mustParsePort(dbPort),
				User:     dbUser,
				Password: []byte(dbPassword),
				Name:     dbName,
				SSLMode:  dbSSLMode,
			},
			DumpFilePath: dumpPath,
		}

		dumpOutput, err := shell.Dump(ctx, dumpInput)
		require.NoError(t, err)
		assert.Greater(t, dumpOutput.UncompressedSize, int64(0))
		assert.Greater(t, dumpOutput.CompressedSize, int64(0))

		// Verify dump directory was created
		_, err = os.Stat(dumpPath)
		require.NoError(t, err)
	}

	// Drop the test table to simulate a restore scenario
	_, err = db.Exec("DROP TABLE IF EXISTS test_users")
	require.NoError(t, err)

	// Verify table is gone
	var tableCount int
	err = db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = 'test_users'", dbName).Scan(&tableCount)
	require.NoError(t, err)
	assert.Equal(t, 0, tableCount)

	// Now restore from the dump
	restoreInput := RestoreInput{
		DatabaseSpec: DatabaseSpec{
			Host:     dbHost,
			Port:     mustParsePort(dbPort),
			User:     dbUser,
			Password: []byte(dbPassword),
			Name:     dbName,
			SSLMode:  dbSSLMode,
		},
		DumpFilePath: dumpPath,
	}

	err = shell.RestoreDump(ctx, restoreInput)
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
