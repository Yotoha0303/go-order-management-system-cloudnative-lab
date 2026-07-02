package response

import "go-order-management-system/internal/model"

type OrderDetailResponse struct {
	Order *model.Order       `json:"order"`
	Items []*model.OrderItem `json:"items"`
}
