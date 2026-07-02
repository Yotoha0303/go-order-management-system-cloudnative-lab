package request

type CreateProductRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=100"`
	Description string `json:"description" binding:"max=500"`
	PriceFen    int64  `json:"price_fen" binding:"required,gt=0"`
}
