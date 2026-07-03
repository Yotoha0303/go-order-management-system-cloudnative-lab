package response

import "go-order-management-system/internal/model"

type ProductListResponse struct {
	Products []*model.Product `json:"products"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}
