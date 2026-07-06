package assistant

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusFinished  OrderStatus = "finished"
	OrderStatusCancelled OrderStatus = "cancelled"
)

func (s OrderStatus) Valid() bool {
	switch s {
	case OrderStatusPending,
		OrderStatusPaid,
		OrderStatusFinished,
		OrderStatusCancelled:
		return true
	default:
		return false
	}
}

type OrderStatusCount struct {
	Status OrderStatus `json:"status"`
	Count  int64       `json:"count"`
}
