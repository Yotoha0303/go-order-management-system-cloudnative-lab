package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"go-order-management-system/config"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func TestMigrationsBackfillExistingUserRole(t *testing.T) {
	if os.Getenv("RUN_MYSQL_TEST") != "1" {
		t.Skip("skip MySQL migration test; set RUN_MYSQL_TEST=1 to run")
	}
	goosePath, err := exec.LookPath("goose")
	if err != nil {
		t.Fatal("goose is required for migration integration tests")
	}

	_ = godotenv.Load("../../.env")
	cfg, err := config.LoadConfig("../../config.yml")
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	password := os.Getenv("MYSQL_TEST_PASSWORD")
	if password == "" {
		t.Fatal("MYSQL_TEST_PASSWORD is required")
	}

	databaseName := fmt.Sprintf("go_order_migration_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	driverConfig := mysqldriver.Config{
		User:      cfg.MySQL.User,
		Passwd:    password,
		Net:       "tcp",
		Addr:      net.JoinHostPort(cfg.MySQL.Host, cfg.MySQL.Port),
		ParseTime: true,
		Loc:       time.Local,
	}
	adminDB, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open admin MySQL failed: %v", err)
	}
	t.Cleanup(func() { _ = adminDB.Close() })
	if err := adminDB.Ping(); err != nil {
		t.Fatalf("ping admin MySQL failed: %v", err)
	}
	if _, err := adminDB.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); err != nil {
		t.Fatalf("create isolated migration database failed: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminDB.Exec("DROP DATABASE IF EXISTS `" + databaseName + "`")
	})

	driverConfig.DBName = databaseName
	migrationDSN := driverConfig.FormatDSN()
	runGoose(t, goosePath, migrationDSN, "up-to", "10")

	testDB, err := sql.Open("mysql", migrationDSN)
	if err != nil {
		t.Fatalf("open migration database failed: %v", err)
	}
	defer testDB.Close()
	if _, err := testDB.Exec(`
		INSERT INTO users (username, password_hash, nickname, status)
		VALUES ('migration-existing-user', 'not-used', 'Existing User', 1)
	`); err != nil {
		t.Fatalf("seed existing user failed: %v", err)
	}

	runGoose(t, goosePath, migrationDSN, "up")

	var roleName string
	err = testDB.QueryRow(`
		SELECT r.role_name
		FROM users AS u
		INNER JOIN user_roles AS ur ON ur.user_id = u.id
		INNER JOIN roles AS r ON r.id = ur.role_id
		WHERE u.username = 'migration-existing-user'
	`).Scan(&roleName)
	if err != nil {
		t.Fatalf("query backfilled user role failed: %v", err)
	}
	if roleName != "user" {
		t.Fatalf("expected existing user role %q, got %q", "user", roleName)
	}

	var foreignKeyCount int
	err = testDB.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.REFERENTIAL_CONSTRAINTS
		WHERE CONSTRAINT_SCHEMA = ? AND TABLE_NAME = 'user_roles'
	`, databaseName).Scan(&foreignKeyCount)
	if err != nil {
		t.Fatalf("query user_roles foreign keys failed: %v", err)
	}
	if foreignKeyCount != 2 {
		t.Fatalf("expected 2 user_roles foreign keys, got %d", foreignKeyCount)
	}

	var aiCallLogColumnCount int
	err = testDB.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = 'ai_call_logs'
	`, databaseName).Scan(&aiCallLogColumnCount)
	if err != nil {
		t.Fatalf("query ai_call_logs columns failed: %v", err)
	}
	if aiCallLogColumnCount != 14 {
		t.Fatalf("expected 14 ai_call_logs columns, got %d", aiCallLogColumnCount)
	}

	var aiCallLogIndexCount int
	err = testDB.QueryRow(`
		SELECT COUNT(DISTINCT INDEX_NAME)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ?
		  AND TABLE_NAME = 'ai_call_logs'
		  AND INDEX_NAME IN (
		      'uk_ai_call_logs_request_id',
		      'idx_ai_call_logs_user_created',
		      'idx_ai_call_logs_status_created'
		  )
	`, databaseName).Scan(&aiCallLogIndexCount)
	if err != nil {
		t.Fatalf("query ai_call_logs indexes failed: %v", err)
	}
	if aiCallLogIndexCount != 3 {
		t.Fatalf("expected 3 ai_call_logs indexes, got %d", aiCallLogIndexCount)
	}

	var orderAssistantIndexCount int
	err = testDB.QueryRow(`
		SELECT COUNT(DISTINCT INDEX_NAME)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ?
		  AND TABLE_NAME = 'orders'
		  AND INDEX_NAME = 'idx_orders_created_at_status'
	`, databaseName).Scan(&orderAssistantIndexCount)
	if err != nil {
		t.Fatalf("query assistant order index failed: %v", err)
	}
	if orderAssistantIndexCount != 1 {
		t.Fatalf("expected assistant order index, got %d", orderAssistantIndexCount)
	}

	var orderTimeoutColumnCount int
	err = testDB.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = 'order_timeout_outbox'
	`, databaseName).Scan(&orderTimeoutColumnCount)
	if err != nil {
		t.Fatalf("query order timeout outbox columns failed: %v", err)
	}
	if orderTimeoutColumnCount != 11 {
		t.Fatalf("expected 11 order timeout outbox columns, got %d", orderTimeoutColumnCount)
	}

	var orderTimeoutForeignKeyCount int
	err = testDB.QueryRow(`
		SELECT COUNT(*)
		FROM information_schema.REFERENTIAL_CONSTRAINTS
		WHERE CONSTRAINT_SCHEMA = ?
		  AND TABLE_NAME = 'order_timeout_outbox'
		  AND CONSTRAINT_NAME = 'fk_order_timeout_outbox_order'
	`, databaseName).Scan(&orderTimeoutForeignKeyCount)
	if err != nil {
		t.Fatalf("query order timeout outbox foreign key failed: %v", err)
	}
	if orderTimeoutForeignKeyCount != 1 {
		t.Fatalf("expected order timeout outbox foreign key, got %d", orderTimeoutForeignKeyCount)
	}

	runGoose(t, goosePath, migrationDSN, "down-to", "0")
}

func runGoose(t *testing.T, goosePath, dsn string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	commandArgs := []string{"-dir", "..", "mysql", dsn}
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(ctx, goosePath, commandArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("goose %v failed: %v\n%s", args, err, output)
	}
}
