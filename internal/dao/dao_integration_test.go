package dao_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"go-order-management-system/config"
	"go-order-management-system/internal/dao"
	"go-order-management-system/internal/model"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestCriticalDAOQueries(t *testing.T) {
	db := setupIsolatedDAOTestDB(t)
	ctx := context.Background()

	adminRole := &model.Role{RoleName: model.RoleAdmin, Description: "administrator"}
	userRole := &model.Role{RoleName: model.RoleUser, Description: "regular user"}
	if err := db.Create([]*model.Role{adminRole, userRole}).Error; err != nil {
		t.Fatalf("seed roles failed: %v", err)
	}
	owner := &model.User{Username: "dao-owner", PasswordHash: "not-used", Nickname: "owner", Status: model.UserStatusActive}
	other := &model.User{Username: "dao-other", PasswordHash: "not-used", Nickname: "other", Status: model.UserStatusActive}
	if err := db.Create([]*model.User{owner, other}).Error; err != nil {
		t.Fatalf("seed users failed: %v", err)
	}

	t.Run("role query reflects database changes", func(t *testing.T) {
		if err := dao.CreateUserRole(ctx, db, owner.ID, userRole.ID); err != nil {
			t.Fatalf("assign user role failed: %v", err)
		}
		allowed, err := dao.UserHasRole(ctx, db, owner.ID, model.RoleAdmin)
		if err != nil {
			t.Fatalf("check admin role failed: %v", err)
		}
		if allowed {
			t.Fatal("expected regular user not to have admin role")
		}

		if err := db.Model(&model.UserRole{}).
			Where("user_id = ?", owner.ID).
			Update("role_id", adminRole.ID).Error; err != nil {
			t.Fatalf("promote user failed: %v", err)
		}
		allowed, err = dao.UserHasRole(ctx, db, owner.ID, model.RoleAdmin)
		if err != nil {
			t.Fatalf("check promoted role failed: %v", err)
		}
		if !allowed {
			t.Fatal("expected role change to take effect immediately")
		}
		roles, err := dao.ListRoleNamesByUserID(ctx, db, owner.ID)
		if err != nil {
			t.Fatalf("list role names failed: %v", err)
		}
		if len(roles) != 1 || roles[0] != model.RoleAdmin {
			t.Fatalf("unexpected roles: %v", roles)
		}
	})

	t.Run("inventory deduction never overdraws stock", func(t *testing.T) {
		inventory := &model.Inventory{ProductID: 1001, StockQuantity: 5}
		if err := db.Create(inventory).Error; err != nil {
			t.Fatalf("seed inventory failed: %v", err)
		}
		rows, err := dao.DeductInventory(ctx, db, inventory.ProductID, 6)
		if err != nil {
			t.Fatalf("deduct excessive inventory failed: %v", err)
		}
		if rows != 0 {
			t.Fatalf("expected excessive deduction to affect 0 rows, got %d", rows)
		}

		rows, err = dao.DeductInventory(ctx, db, inventory.ProductID, 3)
		if err != nil {
			t.Fatalf("deduct inventory failed: %v", err)
		}
		if rows != 1 {
			t.Fatalf("expected valid deduction to affect 1 row, got %d", rows)
		}
		stored, err := dao.GetInventoryByProductID(ctx, db, inventory.ProductID)
		if err != nil {
			t.Fatalf("query inventory failed: %v", err)
		}
		if stored.StockQuantity != 2 {
			t.Fatalf("expected stock 2, got %d", stored.StockQuantity)
		}
	})

	t.Run("order queries enforce ownership", func(t *testing.T) {
		order := &model.Order{
			UserID:         owner.ID,
			OrderNo:        "DAO-ORDER-1",
			TotalAmountFen: 100,
			Status:         model.OrderStatusPending,
		}
		if err := dao.CreateOrder(ctx, db, order); err != nil {
			t.Fatalf("seed order failed: %v", err)
		}
		if _, err := dao.GetOrderByID(ctx, db, other.ID, order.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("expected another user not to find order, got %v", err)
		}
		rows, err := dao.PatchOrderStatus(
			ctx,
			db,
			other.ID,
			order.ID,
			model.OrderStatusPending,
			model.OrderStatusPaid,
			"paid_at",
		)
		if err != nil {
			t.Fatalf("patch another user's order failed: %v", err)
		}
		if rows != 0 {
			t.Fatalf("expected ownership mismatch to affect 0 rows, got %d", rows)
		}
	})

}

func setupIsolatedDAOTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	if os.Getenv("RUN_MYSQL_TEST") != "1" {
		t.Skip("skip MySQL integration test; set RUN_MYSQL_TEST=1 to run")
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

	databaseName := fmt.Sprintf("go_order_dao_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	driverConfig := mysqldriver.Config{
		User:      cfg.MySQL.User,
		Passwd:    password,
		Net:       "tcp",
		Addr:      net.JoinHostPort(cfg.MySQL.Host, cfg.MySQL.Port),
		ParseTime: true,
		Loc:       time.Local,
	}
	adminDB, err := gorm.Open(gormmysql.Open(driverConfig.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect admin MySQL failed: %v", err)
	}
	adminSQL, err := adminDB.DB()
	if err != nil {
		t.Fatalf("get admin sql DB failed: %v", err)
	}
	if err := adminDB.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci").Error; err != nil {
		_ = adminSQL.Close()
		t.Fatalf("create isolated database failed: %v", err)
	}

	driverConfig.DBName = databaseName
	testDB, err := gorm.Open(gormmysql.Open(driverConfig.FormatDSN()), &gorm.Config{TranslateError: true})
	if err != nil {
		_ = adminDB.Exec("DROP DATABASE IF EXISTS `" + databaseName + "`").Error
		_ = adminSQL.Close()
		t.Fatalf("connect isolated database failed: %v", err)
	}
	testSQL, err := testDB.DB()
	if err != nil {
		t.Fatalf("get isolated sql DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = testSQL.Close()
		_ = adminDB.Exec("DROP DATABASE IF EXISTS `" + databaseName + "`").Error
		_ = adminSQL.Close()
	})

	if err := testDB.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.UserRole{},
		&model.Inventory{},
		&model.Order{},
		&model.Product{},
		&model.OrderTimeoutOutbox{},
	); err != nil {
		t.Fatalf("migrate isolated database failed: %v", err)
	}
	return testDB
}
