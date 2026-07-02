package service

import (
	"context"
	"errors"
	"go-order-management-system/internal/dao"
	"go-order-management-system/internal/model"
	"go-order-management-system/internal/request"

	"gorm.io/gorm"
)

const (
	addInventoryRemarkPrefix  = "手动入库：补充"
	initInventoryRemarkPrefix = "初始化库存："
)

func (p *InventoryService) InitInventory(ctx context.Context, req *request.InitInventoryRequest) error {
	if req.StockQuantity == nil {
		return ErrInvalidStockQuantity
	}

	if *req.StockQuantity < 0 {
		return ErrInvalidStockQuantity
	}

	product, err := dao.GetProductByID(ctx, p.db, req.ProductID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrProductNotFound
		}
		return err
	}

	data, err := dao.GetInventoryByProductID(ctx, p.db, req.ProductID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if data.ID != 0 {
		return ErrInitInventoryExists
	}

	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inventory := &model.Inventory{
			ProductID:     product.ID,
			StockQuantity: *req.StockQuantity,
		}

		if err := dao.InitInventory(ctx, tx, inventory); err != nil {
			return ErrInitInventoryFailed
		}

		log := &model.StockLog{
			ProductID:      product.ID,
			BeforeQuantity: 0,
			AfterQuantity:  *req.StockQuantity,
			ChangeQuantity: *req.StockQuantity,
			BizType:        model.StockBizInit,
			Remark:         initInventoryRemarkPrefix + product.Name,
		}

		if err := dao.CreateStockLog(ctx, tx, log); err != nil {
			return ErrCreateStockLogFailed
		}
		return nil
	})
}

func (p *InventoryService) GetInventoryByProductID(ctx context.Context, productID int64) (*model.Inventory, error) {
	inventory, err := dao.GetInventoryByProductID(ctx, p.db, productID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInventoryNotFound
		}
		return nil, err
	}
	return inventory, nil
}

func (p *InventoryService) AddInventory(ctx context.Context, req request.AddInventoryRequest) error {
	if req.Quantity <= 0 {
		return ErrInvalidAddQuantity
	}

	return p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {

		inventory, err := dao.GetInventoryByProductIDForUpdate(ctx, tx, req.ProductID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInventoryNotFound
			}
			return err
		}

		product, err := dao.GetProductByID(ctx, tx, req.ProductID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrProductNotFound
			}
			return err
		}

		beforeQuantity := inventory.StockQuantity
		afterQuantity := beforeQuantity + req.Quantity

		if err := dao.UpdateInventoryStockQuantity(ctx, tx, req.ProductID, afterQuantity); err != nil {
			return err
		}

		log := &model.StockLog{
			ProductID:      req.ProductID,
			BeforeQuantity: beforeQuantity,
			AfterQuantity:  afterQuantity,
			ChangeQuantity: req.Quantity,
			BizType:        model.StockBizManualAdd,
			Remark:         addInventoryRemarkPrefix + product.Name,
		}

		err = dao.CreateStockLog(ctx, tx, log)
		if err != nil {
			return ErrCreateStockLogFailed
		}
		return nil
	})
}
