package request

type InitInventoryRequest struct {
	ProductID     int64  `json:"product_id" binding:"required,gt=0"`
	StockQuantity *int64 `json:"stock_quantity" binding:"required,gte=0"`
}

type AddInventoryRequest struct {
	ProductID int64 `json:"product_id" binding:"required,gt=0"`
	Quantity  int64 `json:"quantity" binding:"required,gt=0"`
}
