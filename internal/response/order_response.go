package response

import "go-order-management-system/internal/model"

type OrderDetailResponse struct {
	Order *model.Order       `json:"order"`
	Items []*model.OrderItem `json:"items"`
}

type OrderListResponse struct {
	Orders   []*model.Order `json:"orders"`
	Total    int64          `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}
