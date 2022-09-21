package backend

import (
	"context"

	"github.com/google/uuid"
)

type Activities struct{}

func (aa *Activities) ClarifyInitialData(ctx context.Context, orderInitialData OrderInitialData) (Order, error) {
	return Order{}, nil
}

func (aa *Activities) CreateOrder(ctx context.Context, order Order) error {
	return nil
}

func (aa *Activities) NotifyUserClientNewOrderCreated(ctx context.Context, order Order) error {
	return nil
}

func (aa *Activities) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, status string) error {
	return nil
}

func (aa *Activities) NotifyUserClientNewOrderStatus(ctx context.Context, orderID uuid.UUID, status string) error {
	return nil
}
