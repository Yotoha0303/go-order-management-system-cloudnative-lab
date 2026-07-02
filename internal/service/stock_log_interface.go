package service

import (
	"gorm.io/gorm"
)

type StockLogService struct {
	db *gorm.DB
}

func NewStockLogService(db *gorm.DB) *StockLogService {
	return &StockLogService{
		db: db,
	}
}
