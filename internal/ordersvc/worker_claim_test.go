package ordersvc

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	gomysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestClaimPendingUsesExclusiveLeases(t *testing.T) {
	db := openOutboxLeaseTestDB(t)
	now := time.Now()
	for orderID := int64(1); orderID <= 3; orderID++ {
		if err := db.Exec(`
			INSERT INTO order_timeout_outbox_v2
				(order_id, due_at, status, attempts, last_error, lease_owner, lease_until, next_attempt_at, created_at, updated_at)
			VALUES (?, ?, ?, 0, '', '', NULL, ?, ?, ?)
		`, orderID, now.Add(time.Minute), OutboxPending, now, now, now).Error; err != nil {
			t.Fatalf("insert outbox event: %v", err)
		}
	}

	workerOne := &Worker{
		cfg: WorkerConfig{WorkerID: "worker-one", BatchSize: 2, LeaseDuration: time.Minute},
		db:  db,
	}
	workerTwo := &Worker{
		cfg: WorkerConfig{WorkerID: "worker-two", BatchSize: 2, LeaseDuration: time.Minute},
		db:  db,
	}

	first, err := workerOne.claimPending(context.Background())
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	second, err := workerTwo.claimPending(context.Background())
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(first) != 2 || len(second) != 1 {
		t.Fatalf("unexpected claim sizes: first=%d second=%d", len(first), len(second))
	}

	seen := make(map[uint64]struct{}, 3)
	for _, event := range append(first, second...) {
		if _, exists := seen[event.ID]; exists {
			t.Fatalf("event %d was claimed twice", event.ID)
		}
		seen[event.ID] = struct{}{}
	}

	var ownerCounts []struct {
		LeaseOwner string
		Count      int64
	}
	if err := db.Table(TimeoutOutbox{}.TableName()).
		Select("lease_owner, count(*) AS count").
		Group("lease_owner").
		Order("lease_owner").
		Scan(&ownerCounts).Error; err != nil {
		t.Fatalf("query lease owners: %v", err)
	}
	if len(ownerCounts) != 2 || ownerCounts[0].Count+ownerCounts[1].Count != 3 {
		t.Fatalf("unexpected lease distribution: %#v", ownerCounts)
	}

	if err := db.Table(TimeoutOutbox{}.TableName()).
		Where("id > 0").
		Update("lease_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatalf("expire leases: %v", err)
	}
	workerThree := &Worker{
		cfg: WorkerConfig{WorkerID: "worker-three", BatchSize: 3, LeaseDuration: time.Minute},
		db:  db,
	}
	reclaimed, err := workerThree.claimPending(context.Background())
	if err != nil {
		t.Fatalf("reclaim expired leases: %v", err)
	}
	if len(reclaimed) != 3 {
		t.Fatalf("expected 3 reclaimed events, got %d", len(reclaimed))
	}
}

func openOutboxLeaseTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("RUN_MYSQL_TEST") != "1" {
		t.Skip("set RUN_MYSQL_TEST=1 to run MySQL integration tests")
	}

	host := outboxTestEnvOr("MYSQL_TEST_HOST", "127.0.0.1")
	port := outboxTestEnvOr("MYSQL_TEST_PORT", "3306")
	password := os.Getenv("MYSQL_TEST_PASSWORD")
	databaseName := fmt.Sprintf("go_order_outbox_lease_%d", time.Now().UnixNano())

	config := gomysql.Config{
		User:            "root",
		Passwd:          password,
		Net:             "tcp",
		Addr:            net.JoinHostPort(host, port),
		ParseTime:       true,
		MultiStatements: true,
	}
	admin, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatalf("open MySQL admin connection: %v", err)
	}
	if _, err := admin.Exec("CREATE DATABASE `" + databaseName + "`"); err != nil {
		_ = admin.Close()
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE IF EXISTS `" + databaseName + "`")
		_ = admin.Close()
	})

	config.DBName = databaseName
	db, err := gorm.Open(gormmysql.Open(config.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open GORM test database: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE order_timeout_outbox_v2 (
			id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			order_id BIGINT NOT NULL,
			due_at DATETIME(3) NOT NULL,
			status VARCHAR(20) NOT NULL,
			attempts INT NOT NULL DEFAULT 0,
			last_error VARCHAR(500) NOT NULL DEFAULT '',
			lease_owner VARCHAR(128) NOT NULL DEFAULT '',
			lease_until DATETIME(3) NULL,
			next_attempt_at DATETIME(3) NOT NULL,
			created_at DATETIME(3) NOT NULL,
			updated_at DATETIME(3) NOT NULL,
			PRIMARY KEY (id),
			UNIQUE KEY uk_order_timeout_outbox_order (order_id),
			KEY idx_order_timeout_outbox_claim (status, next_attempt_at, lease_until)
		) ENGINE=InnoDB
	`).Error; err != nil {
		t.Fatalf("create outbox table: %v", err)
	}
	return db
}

func outboxTestEnvOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
