package model

import "time"

const (
	StockBizInit          int8 = 1 //初始化库存
	StockBizManualAdd     int8 = 2 //手动入库
	StockBizOrderDeduct   int8 = 3 //下单扣减
	StockBizOrderRollback int8 = 4 //订单取消
)

type StockLog struct {
	ID             int64 `gorm:"primaryKey;autoIncrement;type:bigint" json:"id"`
	ProductID      int64 `gorm:"type:bigint;not null;index:idx_stock_logs_product_id" json:"product_id"`
	ChangeQuantity int64 `gorm:"type:bigint;not null" json:"change_quantity"`
	BeforeQuantity int64 `gorm:"type:bigint;not null" json:"before_quantity"`
	AfterQuantity  int64 `gorm:"type:bigint;not null" json:"after_quantity"`

	BizType int8   `gorm:"type:tinyint;not null;index:idx_stock_logs_biz_type" json:"biz_type"`
	BizID   *int64 `gorm:"type:bigint;index:idx_stock_logs_biz_id" json:"biz_id"`

	Remark    string    `gorm:"type:varchar(255);not null;default:''" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
}

func (StockLog) TableName() string {
	return "stock_logs"
}
