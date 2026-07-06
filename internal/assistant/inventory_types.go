package assistant

type LowStockProduct struct {
	ProductID int64  `json:"product_id"`
	Name      string `json:"name"`
	Stock     int64  `json:"stock"`
}
