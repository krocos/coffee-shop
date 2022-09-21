package backend

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/workflow"
)

const (
	orderStatusWaitingForPayment   = "waiting_for_payment"
	orderStatusPaymentTimeout      = "payment_timeout"
	orderStatusPaid                = "paid"
	orderStatusUnsuccessfulPayment = "unsuccessful_payment"
)

type (
	OrderInitialData struct {
		UserID  uuid.UUID
		PointID uuid.UUID
		Items   []*ItemInitialData
	}
	ItemInitialData struct {
		ID       uuid.UUID
		Quantity float64
	}
)

type (
	Order struct {
		ID        uuid.UUID
		Point     Point
		Items     []Item
		User      User
		Status    string
		PINCode   string
		GivenAway bool
	}
	User struct {
		ID   uuid.UUID
		Name string
	}
	Item struct {
		ID       uuid.UUID
		Title    string
		Quantity float64
		Ready    bool
	}
	Point struct {
		ID        uuid.UUID
		Addr      string
		KitchenID uuid.UUID
		CacheID   uuid.UUID
	}
)

type PaymentSignal struct {
	Success bool
}

func OrderWorkflow(ctx workflow.Context, orderInitialData OrderInitialData) error {
	var (
		order Order
		aa    *Activities
	)

	ao := workflow.ActivityOptions{StartToCloseTimeout: time.Hour}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Получаем все референсы для заказа типа пользователя, точки, итемов.
	if err := workflow.ExecuteActivity(ctx, aa.ClarifyInitialData, orderInitialData).Get(ctx, &order); err != nil {
		return err
	}

	// Создаём пинкод для выдачи заказа.
	if err := workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
		return fmt.Sprintf("%04d", rand.Intn(10000))
	}).Get(&order.PINCode); err != nil {
		return err
	}

	// Назначаем оредеру статус, что ожидает оплаты.
	order.Status = orderStatusWaitingForPayment

	// Записываем ордер в базу для клиента пользователя.
	if err := workflow.ExecuteActivity(ctx, aa.CreateOrder, order).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем клиента пользователя, что заказ создан и ожидает оплаты.
	if err := workflow.ExecuteActivity(ctx, aa.NotifyUserClientNewOrderCreated, order).Get(ctx, nil); err != nil {
		return err
	}

	// Ожидаем сигнала об оплате от платёжного интегратора или таймаута.
	paymentSelector := workflow.NewSelector(ctx)

	paymentReceivedSignalChan := workflow.GetSignalChannel(ctx, "payment")
	cancelOrderTimer := workflow.NewTimer(ctx, time.Hour)

	paymentSelector.AddReceive(paymentReceivedSignalChan, func(ch workflow.ReceiveChannel, more bool) {
		var s PaymentSignal
		ch.Receive(ctx, &s)

		if s.Success {
			order.Status = orderStatusPaid
		} else {
			order.Status = orderStatusUnsuccessfulPayment
		}
	})
	paymentSelector.AddFuture(cancelOrderTimer, func(f workflow.Future) {
		// Устанавливаем статус, что оплата просрочена (заказ отменяется и выходим после селекта).
		order.Status = orderStatusPaymentTimeout
	})

	paymentSelector.Select(ctx)

	// Записываем в базу новый статус оредера для клиента пользователя.
	if err := workflow.ExecuteActivity(ctx, aa.UpdateOrderStatus, order.ID, order.Status).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем клиент пользователя, что у ордера сменился статус.
	if err := workflow.ExecuteActivity(ctx, aa.NotifyUserClientNewOrderStatus, order.ID, order.Status).Get(ctx, nil); err != nil {
		return err
	}

	if order.Status != orderStatusPaid {
		// Тут выходим ибо заказ не оплачен.

		// Можно как вариант подумать сделать возможным несколько повторных попыток оплаты заказа.

		return nil
	}

	// Записываем в заказы кухни на точке, какие итемы надо приготовить.

	// Уведомляем клиент кухни точки, что появились новые итемы на приготовку.

	// Записываем в заказах кассы точки, что появился новый ордер и он отдан на кухню точки, что бы
	// оператор кассы знал что заказ готовится, что бы говорить с клиентом, если он рано явился.

	// Уведомляем клиент кассы точки, что есть готовящийся заказ.

	return nil
}
