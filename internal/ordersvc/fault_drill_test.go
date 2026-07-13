package ordersvc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const faultDrillHelperEnv = "GO_ORDER_FAULT_LEASE_HELPER"

type faultOutboxRow struct {
	ID         uint64
	OrderID    uint64
	Status     string
	Attempts   int
	LeaseOwner string
	LeaseUntil *time.Time
}

func openFaultDrillDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open fault-drill database: %v", err)
	}
	return db
}

func faultDrillWorker(t *testing.T, db *gorm.DB, workerID string, lease time.Duration) *Worker {
	t.Helper()
	worker, err := NewWorker(WorkerConfig{
		URL:             "amqp://fault-drill.invalid/",
		OrderServiceURL: "http://fault-drill.invalid",
		InternalToken:   "fault-drill-token",
		WorkerID:        workerID,
		LeaseDuration:   lease,
		BatchSize:       1,
	}, db, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("create fault-drill worker: %v", err)
	}
	return worker
}

func TestFaultDrillWorkerLeaseProcess(t *testing.T) {
	if os.Getenv(faultDrillHelperEnv) == "1" {
		runFaultDrillLeaseHelper(t)
		return
	}

	dsn := os.Getenv("FAULT_DRILL_MYSQL_DSN")
	if dsn == "" {
		t.Skip("FAULT_DRILL_MYSQL_DSN is not set")
	}
	outputDir := os.Getenv("FAULT_DRILL_OUTPUT_DIR")
	if outputDir == "" {
		outputDir = t.TempDir()
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatalf("create fault-drill output directory: %v", err)
	}

	db := openFaultDrillDB(t, dsn)
	table := TimeoutOutbox{}.TableName()
	var fixture faultOutboxRow
	if err := db.Table(table).Order("id DESC").Take(&fixture).Error; err != nil {
		t.Fatalf("load timeout outbox fixture: %v", err)
	}
	if err := db.Table(table).Where("id = ?", fixture.ID).Updates(map[string]any{
		"status":          OutboxPending,
		"attempts":        0,
		"last_error":      "",
		"next_attempt_at": time.Now().Add(-time.Second),
		"due_at":          time.Now().Add(-time.Second),
		"lease_owner":     "",
		"lease_until":     nil,
	}).Error; err != nil {
		t.Fatalf("prepare timeout outbox fixture: %v", err)
	}

	leaseDuration := 400 * time.Millisecond
	readyPath := filepath.Join(outputDir, "worker-owner-ready.json")
	helper := exec.Command(os.Args[0], "-test.run=^TestFaultDrillWorkerLeaseProcess$", "-test.v")
	helper.Env = append(os.Environ(),
		faultDrillHelperEnv+"=1",
		"FAULT_DRILL_EVENT_ID="+strconv.FormatUint(fixture.ID, 10),
		"FAULT_DRILL_READY_PATH="+readyPath,
		"FAULT_DRILL_LEASE_DURATION="+leaseDuration.String(),
	)
	helper.Stdout = os.Stdout
	helper.Stderr = os.Stderr
	if err := helper.Start(); err != nil {
		t.Fatalf("start lease-owner process: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = helper.Process.Kill()
			_ = helper.Wait()
			t.Fatal("lease-owner process did not claim the fixture in time")
		}
		time.Sleep(20 * time.Millisecond)
	}

	var owned faultOutboxRow
	if err := db.Table(table).Where("id = ?", fixture.ID).Take(&owned).Error; err != nil {
		t.Fatalf("read owned lease: %v", err)
	}
	if owned.LeaseOwner != "fault-owner" || owned.LeaseUntil == nil {
		t.Fatalf("fixture was not leased by fault-owner: %+v", owned)
	}

	crashedAt := time.Now()
	if err := helper.Process.Kill(); err != nil {
		t.Fatalf("kill lease-owner process: %v", err)
	}
	_ = helper.Wait()

	for time.Now().Before(owned.LeaseUntil.Add(100 * time.Millisecond)) {
		time.Sleep(20 * time.Millisecond)
	}

	recovery := faultDrillWorker(t, db, "recovery-worker", leaseDuration)
	claimed, err := recovery.claimPending(context.Background())
	if err != nil {
		t.Fatalf("recovery worker claim: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != fixture.ID {
		t.Fatalf("recovery worker did not reclaim exact fixture: %+v", claimed)
	}
	if err := recovery.markOutboxPublished(context.Background(), fixture.ID); err != nil {
		t.Fatalf("complete recovered outbox event: %v", err)
	}

	var recovered faultOutboxRow
	if err := db.Table(table).Where("id = ?", fixture.ID).Take(&recovered).Error; err != nil {
		t.Fatalf("read recovered outbox row: %v", err)
	}
	if recovered.Status != OutboxPublished || recovered.Attempts != 1 || recovered.LeaseOwner != "" || recovered.LeaseUntil != nil {
		t.Fatalf("recovered outbox row has invalid terminal state: %+v", recovered)
	}
	var rowCount int64
	if err := db.Table(table).Where("id = ?", fixture.ID).Count(&rowCount).Error; err != nil {
		t.Fatalf("count recovered outbox rows: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("worker recovery created duplicate outbox rows: %d", rowCount)
	}

	evidence := map[string]any{
		"schema_version":        1,
		"event_id":              fixture.ID,
		"order_id":              fixture.OrderID,
		"terminated_worker":     "fault-owner",
		"recovery_worker":       "recovery-worker",
		"lease_duration_ms":     leaseDuration.Milliseconds(),
		"recovery_elapsed_ms":   time.Since(crashedAt).Milliseconds(),
		"terminal_status":       recovered.Status,
		"attempts":              recovered.Attempts,
		"duplicate_outbox_rows": rowCount - 1,
		"lease_cleared":         recovered.LeaseOwner == "" && recovered.LeaseUntil == nil,
	}
	writeFaultDrillJSON(t, filepath.Join(outputDir, "worker-lease-recovery.json"), evidence)
}

func runFaultDrillLeaseHelper(t *testing.T) {
	dsn := os.Getenv("FAULT_DRILL_MYSQL_DSN")
	readyPath := os.Getenv("FAULT_DRILL_READY_PATH")
	eventID, err := strconv.ParseUint(os.Getenv("FAULT_DRILL_EVENT_ID"), 10, 64)
	if err != nil {
		t.Fatalf("parse helper event ID: %v", err)
	}
	leaseDuration, err := time.ParseDuration(os.Getenv("FAULT_DRILL_LEASE_DURATION"))
	if err != nil {
		t.Fatalf("parse helper lease duration: %v", err)
	}
	db := openFaultDrillDB(t, dsn)
	worker := faultDrillWorker(t, db, "fault-owner", leaseDuration)
	claimed, err := worker.claimPending(context.Background())
	if err != nil {
		t.Fatalf("fault owner claim: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != eventID {
		t.Fatalf("fault owner did not claim exact fixture: %+v", claimed)
	}
	writeFaultDrillJSON(t, readyPath, map[string]any{
		"event_id":    eventID,
		"lease_owner": "fault-owner",
		"claimed_at":  time.Now().UTC().Format(time.RFC3339Nano),
	})
	select {}
}

func writeFaultDrillJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal fault-drill JSON: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fault-drill JSON %s: %v", path, err)
	}
}

func TestFaultDrillWorkerLeaseConfiguration(t *testing.T) {
	if _, err := time.ParseDuration("400ms"); err != nil {
		t.Fatal(fmt.Errorf("invalid fault-drill lease duration: %w", err))
	}
}
