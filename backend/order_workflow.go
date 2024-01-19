package backend

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"go.temporal.io/sdk/workflow"

	"github.com/krocos/coffee-shop/elasticsearch"
	"github.com/krocos/coffee-shop/postgres"
	"github.com/krocos/coffee-shop/sse"
)

const (
	orderStatusWaitingForPayment = "waiting_for_payment"
	orderStatusPaymentTimeout    = "payment_timeout"
	orderStatusPaid              = "paid"
	orderStatusPaymentCanceled   = "payment_canceled"
	orderStatusCooking           = "cooking"
	orderStatusReady             = "ready"
	orderStatusReceived          = "received"
)

const (
	paymentSignalSuccessful   = "successful"
	paymentSignalUnsuccessful = "unsuccessful"
	paymentSignalCanceled     = "canceled"
)

const (
	cacheOrderStatusCooking = "cooking"
	cacheOrderStatusReady   = "ready"
)

type (
	OrderInitialData struct {
		ID      uuid.UUID
		UserID  uuid.UUID
		PointID uuid.UUID
		Items   []ItemInitialData
	}
	ItemInitialData struct {
		ID       uuid.UUID
		Quantity float64
	}
)

type (
	Order struct {
		id         uuid.UUID
		createdAt  time.Time
		status     string
		totalPrice float64
		pinCode    string

		user       *User
		orderItems []*OrderItem
		point      *Point
		logs       []*LogItem
	}
	User struct {
		id   uuid.UUID
		name string
	}
	Point struct {
		id        uuid.UUID
		addr      string
		kitchenID uuid.UUID
		cacheID   uuid.UUID
	}
	OrderItem struct {
		id         uuid.UUID
		title      string
		price      float64
		itemID     uuid.UUID
		quantity   float64
		totalPrice float64
		ready      bool
	}
	LogItem struct {
		id   uuid.UUID
		Text string
	}
)

func (d *OrderInitialData) itemQuantity(itemID uuid.UUID) float64 {
	for _, item := range d.Items {
		if item.ID.String() == itemID.String() {
			return item.Quantity
		}
	}
	return 0.0
}

type (
	PaymentSignal struct {
		Status string
		Reason string
	}
	CookingSignal struct {
		OrderItemID uuid.UUID
	}
	ReceiveSignal struct {
		PINCode string
	}
)

func OrderWorkflow(ctx workflow.Context, initialData OrderInitialData) error {

	processing := newOrderProcessing(ctx, initialData.ID)

	ao := workflow.ActivityOptions{StartToCloseTimeout: time.Hour}
	ctx = workflow.WithActivityOptions(ctx, ao)

	if err := processing.prepareOrder(ctx, initialData); err != nil {
		return err
	}

	if err := processing.processPayment(ctx); err != nil {
		return err
	}

	if processing.order.status != orderStatusPaid {
		// Тут выходим ибо заказ не оплачен.
		return nil
	}

	if err := processing.launchCookingOnPoint(ctx); err != nil {
		return err
	}

	if err := processing.waitForCooking(ctx); err != nil {
		return err
	}

	if err := processing.giveAway(ctx); err != nil {
		return err
	}

	if err := processing.cleanUp(ctx); err != nil {
		return err
	}

	return nil
}

type orderProcessing struct {
	loc *time.Location

	storage    *postgres.Postgres
	sseService *sse.SSE
	search     *elasticsearch.Search

	order *Order
}

func newOrderProcessing(ctx workflow.Context, orderID uuid.UUID) *orderProcessing {
	loc, _ := time.LoadLocation("Asia/Yekaterinburg")

	return &orderProcessing{
		loc: loc,
		order: &Order{
			id:        orderID,
			createdAt: workflow.Now(ctx).In(loc),
		},
	}
}

// prepareOrder создаёт заказ по которому потом дальше работать.
func (p *orderProcessing) prepareOrder(ctx workflow.Context, initialData OrderInitialData) error {
	// Получаем данные пользователя. Тут это может пригодиться, что бы например вместе с этими данными получить
	// ещё и данные для предоставления скидки, например. В данном примере нас интересует только имя пользователя.
	var userData postgres.UserData
	if err := workflow.ExecuteActivity(ctx, p.storage.GetUserData, initialData.UserID).Get(ctx, &userData); err != nil {
		return err
	}

	p.order.user = &User{
		id:   userData.ID,
		name: userData.Name,
	}

	// Получаем данные итемов заказа, что бы можно было рассчитать общую стоимость заказа. Если бы мы имели
	// скидоную систему, то можно было бы после загрузки данных пользователя добавить и скидку, например.
	itemIDs := lo.Map(initialData.Items, func(item ItemInitialData, _ int) uuid.UUID { return item.ID })
	var itemsData []postgres.ItemData
	if err := workflow.ExecuteActivity(ctx, p.storage.GetItemsData, itemIDs).Get(ctx, &itemsData); err != nil {
		return err
	}

	p.order.orderItems = make([]*OrderItem, 0)
	for _, data := range itemsData {
		var orderItemID uuid.UUID

		// Создаём новый идентификатор товара в заказе.
		if err := workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} { return uuid.New() }).Get(&orderItemID); err != nil {
			return err
		}

		quantity := initialData.itemQuantity(data.ID)

		p.order.orderItems = append(p.order.orderItems, &OrderItem{
			id:         orderItemID,
			title:      data.Title,
			price:      data.Price,
			itemID:     data.ID,
			quantity:   quantity,
			totalPrice: data.Price * quantity,
		})
	}

	// Получаем данные точки. Так как нам в основном надо только адрес и идентификаторы
	// терминалов кухни и кассы (для данного примера, то только их и получаем).
	var pointData postgres.PointData
	if err := workflow.ExecuteActivity(ctx, p.storage.GetPointData, initialData.PointID).Get(ctx, &pointData); err != nil {
		return err
	}

	p.order.point = &Point{
		id:        pointData.ID,
		addr:      pointData.Addr,
		kitchenID: pointData.KitchenID,
		cacheID:   pointData.CacheID,
	}

	// Создаём пинкод для выдачи заказа.
	if err := workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
		return fmt.Sprintf("%04d", rand.Intn(10000))
	}).Get(&p.order.pinCode); err != nil {
		return err
	}

	// Назначаем оредеру статус, что ожидает оплаты.
	p.order.status = orderStatusWaitingForPayment

	// Считаем общую сумму заказа.
	for _, item := range p.order.orderItems {
		p.order.totalPrice += item.totalPrice
	}

	// Записываем ордер в базу для длинной истории.
	createOrderParams := postgres.OrderParams{
		ID:         p.order.id,
		CreatedAt:  p.order.createdAt,
		Status:     p.order.status,
		TotalPrice: p.order.totalPrice,
		PINCode:    p.order.pinCode,
		UserID:     p.order.user.id,
		PointID:    p.order.point.id,
		Items:      make([]postgres.OrderItemParams, 0),
	}
	for _, orderItem := range p.order.orderItems {
		createOrderParams.Items = append(createOrderParams.Items, postgres.OrderItemParams{
			ID:         orderItem.id,
			Title:      orderItem.title,
			Price:      orderItem.price,
			ItemID:     orderItem.itemID,
			Quantity:   orderItem.quantity,
			TotalPrice: orderItem.totalPrice,
		})
	}
	if err := workflow.ExecuteActivity(ctx, p.storage.CreateOrder, createOrderParams).Get(ctx, nil); err != nil {
		return err
	}

	// Индексируем ордер для быстрой выдачи на клиент пользователя (истоиря ордеров бесконечно пополняется).

	searchOrder := elasticsearch.Order{
		ID:         p.order.id.String(),
		CreatedAt:  p.order.createdAt.Format(time.RFC3339),
		Status:     p.order.status,
		TotalPrice: p.order.totalPrice,
		PINCode:    p.order.pinCode,
		User: &elasticsearch.User{
			ID:   p.order.user.id.String(),
			Name: p.order.user.name,
		},
		Point: &elasticsearch.Point{
			ID:        p.order.point.id.String(),
			Addr:      p.order.point.addr,
			KitchenID: p.order.point.kitchenID.String(),
			CacheID:   p.order.point.cacheID.String(),
		},
	}

	for _, item := range p.order.orderItems {
		searchOrder.Items = append(searchOrder.Items, &elasticsearch.OrderItem{
			ID:         item.id.String(),
			Title:      item.title,
			Price:      item.price,
			ItemID:     item.itemID.String(),
			Quantity:   item.quantity,
			TotalPrice: item.totalPrice,
			OrderID:    p.order.id.String(),
		})
	}

	for _, logItem := range p.order.logs {
		searchOrder.LogItems = append(searchOrder.LogItems, &elasticsearch.LogItem{
			ID:      logItem.id.String(),
			Text:    logItem.Text,
			OrderID: p.order.id.String(),
		})
	}

	if err := workflow.ExecuteActivity(ctx, p.search.IndexOrder, p.order.id, searchOrder, true).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем клиента пользователя, что заказ создан и ожидает оплаты.
	if err := workflow.ExecuteActivity(ctx, p.sseService.SendNotification,
		sse.NewOrderListUpdatedEvent().ForUser().WithID(p.order.user.id)).Get(ctx, nil); err != nil {

		return err
	}

	return nil
}

// processPayment ожидает сигнала об оплате от платёжного интегратора, таймаута
// или отмены заказа.
func (p *orderProcessing) processPayment(ctx workflow.Context) error {
	// Ожидаем сигнала об оплате от платёжного интегратора, таймаута или отмены заказа.

	paymentSignals := workflow.GetSignalChannel(ctx, "payment_signals")

	for {
		if p.order.status != orderStatusWaitingForPayment {
			break
		}

		paymentSelector := workflow.NewSelector(ctx)

		paymentTimeout := workflow.NewTimer(ctx, time.Hour)

		// Если будет неудачный платеж, то мы запишем причину сюда, которая пришла от платёжного агрегатора.
		var unsuccessfulPaymentReason string

		paymentSelector.AddReceive(paymentSignals, func(ch workflow.ReceiveChannel, more bool) {
			var s PaymentSignal
			ch.Receive(ctx, &s)

			switch s.Status {
			case paymentSignalSuccessful:
				p.order.status = orderStatusPaid
			case paymentSignalUnsuccessful:
				unsuccessfulPaymentReason = s.Reason
			case paymentSignalCanceled:
				p.order.status = orderStatusPaymentCanceled
			}
		})
		paymentSelector.AddFuture(paymentTimeout, func(f workflow.Future) {
			// Устанавливаем статус, что оплата просрочена (заказ отменяется и выходим после селекта).
			p.order.status = orderStatusPaymentTimeout
		})

		paymentSelector.Select(ctx)

		// Если был неудачный платеж, то надо это записать для клиента
		// пользователя, что бы можно было отобразить это на фронте.
		if unsuccessfulPaymentReason != "" {
			var (
				logID uuid.UUID
				text  = fmt.Sprintf("Оплата: %s", unsuccessfulPaymentReason)
			)

			// Создаём новый идентификатор для записи лога.
			if err := workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
				return uuid.New()
			}).Get(&logID); err != nil {
				return err
			}

			p.order.logs = append(p.order.logs, &LogItem{
				id:   logID,
				Text: text,
			})

			// Добавляем запись лога в базу данных.
			if err := workflow.ExecuteActivity(ctx, p.storage.LogUnsuccessfulPayment, postgres.LogUnsuccessfulPaymentParams{
				ID:      logID,
				OrderID: p.order.id,
				Reason:  text,
			}).Get(ctx, nil); err != nil {
				return err
			}

			// Обновляем логи в индексе.

			logs := make([]*elasticsearch.LogItem, 0)
			for _, l := range p.order.logs {
				logs = append(logs, &elasticsearch.LogItem{
					ID:      l.id.String(),
					Text:    l.Text,
					OrderID: p.order.id.String(),
				})
			}

			if err := workflow.ExecuteActivity(ctx, p.search.UpdateOrder, p.order.id, &elasticsearch.Order{LogItems: logs}, true).Get(ctx, nil); err != nil {
				return err
			}

			// Уведомляем пользователя о новых логах.
			if err := workflow.ExecuteActivity(ctx, p.sseService.SendNotification,
				sse.NewUnsuccessfulPayAttemptEvent().ForUser().WithID(p.order.user.id)).Get(ctx, nil); err != nil {

				return err
			}

			unsuccessfulPaymentReason = ""
		}
	}

	// Записываем измеение статуса ордера в базу данных для клинета пользователя.
	if err := workflow.ExecuteActivity(ctx, p.storage.UpdateOrderStatus, p.order.id, p.order.status).Get(ctx, nil); err != nil {
		return err
	}

	// Ообновляем статус заказа в индексе.
	if err := workflow.ExecuteActivity(ctx, p.search.UpdateOrder, p.order.id, &elasticsearch.Order{Status: p.order.status}, true).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем клиента пользователя, что статус заказа изменился.
	if err := workflow.ExecuteActivity(ctx, p.sseService.SendNotification,
		sse.NewOrderListUpdatedEvent().ForUser().WithID(p.order.user.id)).Get(ctx, nil); err != nil {

		return err
	}

	return nil
}

// launchCookingOnPoint запускаем процесс готовки на точке, отправляем данные
// для готовки на её кухню и информацию для кассира.
func (p *orderProcessing) launchCookingOnPoint(ctx workflow.Context) error {
	// Записываем в заказы кухни на точке, какие итемы надо приготовить.
	cookingParams := postgres.AddItemsForCookingParams{
		KitchenID: p.order.point.kitchenID,
		OrderID:   p.order.id,
		Items:     make([]postgres.ItemForCooking, 0),
	}
	for _, item := range p.order.orderItems {
		cookingParams.Items = append(cookingParams.Items, postgres.ItemForCooking{
			ID:       item.id,
			Title:    item.title,
			Quantity: item.quantity,
		})
	}
	if err := workflow.ExecuteActivity(ctx, p.storage.AddItemsForKitchen, cookingParams).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем клиент кухни точки, что появились новые итемы на приготовку.
	if err := workflow.ExecuteActivity(ctx, p.sseService.SendNotification,
		sse.NewItemListUpdatedEvent().ForKitchen().WithID(p.order.point.kitchenID)).Get(ctx, nil); err != nil {

		return err
	}

	// Записываем в заказах кассы точки, что появился новый ордер и он отдан на кухню точки, что бы
	// оператор кассы знал что заказ готовится, что бы говорить с клиентом, если он рано явился.

	checkList := make([]string, 0)
	for _, item := range p.order.orderItems {
		checkList = append(checkList, fmt.Sprintf("%s %.0f шт.", item.title, item.quantity))
	}

	if err := workflow.ExecuteActivity(ctx, p.storage.AddNewOrderForCache, postgres.AddNewOrderForCacheParams{
		ID:               p.order.id,
		CacheID:          p.order.point.cacheID,
		OrderID:          p.order.id,
		UserName:         p.order.user.name,
		Status:           cacheOrderStatusCooking,
		ReadinessPercent: 0,
		CheckList:        strings.Join(checkList, ", "),
	}).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем клиент кассы точки, что есть готовящийся заказ.
	if err := workflow.ExecuteActivity(ctx, p.sseService.SendNotification,
		sse.NewOrderListUpdatedEvent().ForCache().WithID(p.order.point.cacheID)).Get(ctx, nil); err != nil {

		return err
	}

	p.order.status = orderStatusCooking

	// Изменить статус заказа для клиента пользователя, что заказ готовится.
	if err := workflow.ExecuteActivity(ctx, p.storage.UpdateOrderStatus, p.order.id, p.order.status).Get(ctx, nil); err != nil {
		return err
	}

	// Ообновляем статус заказа в индексе.
	if err := workflow.ExecuteActivity(ctx, p.search.UpdateOrder, p.order.id, &elasticsearch.Order{Status: p.order.status}, true).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомить клиент пользователя, что изменился статус заказа.
	if err := workflow.ExecuteActivity(ctx, p.sseService.SendNotification,
		sse.NewOrderListUpdatedEvent().ForUser().WithID(p.order.user.id)).Get(ctx, nil); err != nil {

		return err
	}

	return nil
}

// waitForCooking ожидание готовности заказа.
func (p *orderProcessing) waitForCooking(ctx workflow.Context) error {
	cookingSignals := workflow.GetSignalChannel(ctx, "cooking_signals")

	for {
		// Тут сделано через селектор, хотя, можно было бы просто слушать
		// канал игналов, но так будет проще потом улучшить петлю сигналов.

		cookingSelector := workflow.NewSelector(ctx)

		var cookedOrderItemID uuid.UUID

		cookingSelector.AddReceive(cookingSignals, func(ch workflow.ReceiveChannel, more bool) {
			var s CookingSignal
			ch.Receive(ctx, &s)

			for _, orderItem := range p.order.orderItems {
				if orderItem.id.String() == s.OrderItemID.String() {
					orderItem.ready = true
					cookedOrderItemID = s.OrderItemID
				}
			}
		})

		cookingSelector.Select(ctx)

		var (
			ready    int
			notReady int
		)

		for _, orderItem := range p.order.orderItems {
			if orderItem.ready {
				ready++
			} else {
				notReady++
			}
		}

		var readyPercent int

		if notReady == 0 {
			readyPercent = 100
		} else {
			readyPercent = ready * 100 / (ready + notReady)
		}

		// Удаляем приготовленный итем с кухни, что бы не отображался на экране клиента кухни.
		if err := workflow.ExecuteActivity(ctx, p.storage.RemoveKitchenCookItemAsReady, cookedOrderItemID).Get(ctx, nil); err != nil {
			return err
		}

		// Уведомляем клиент кухни, что бы перезагузил заказы.
		if err := workflow.ExecuteActivity(ctx, p.sseService.SendNotification,
			sse.NewItemListUpdatedEvent().ForKitchen().WithID(p.order.point.kitchenID)).Get(ctx, nil); err != nil {

			return err
		}

		// Обновляем процент готовности на кассе.
		if err := workflow.ExecuteActivity(ctx, p.storage.UpdateCacheOrderReadinessPercent, p.order.id, readyPercent).Get(ctx, nil); err != nil {
			return err
		}

		if readyPercent == 100 {
			if err := workflow.ExecuteActivity(ctx, p.storage.UpdateCacheOrderStatus, p.order.id, cacheOrderStatusReady).Get(ctx, nil); err != nil {
				return err
			}
		}

		// Уведомляем клиент кассы, что изменился процент готовности заказа.
		if err := workflow.ExecuteActivity(ctx, p.sseService.SendNotification,
			sse.NewOrderListUpdatedEvent().ForCache().WithID(p.order.point.cacheID)).Get(ctx, nil); err != nil {

			return err
		}

		if readyPercent != 100 {
			continue
		}

		p.order.status = orderStatusReady

		// Если заказ готов, то обновляем статус заказа для клиента пользователя.
		if err := workflow.ExecuteActivity(ctx, p.storage.UpdateOrderStatus, p.order.id, p.order.status).Get(ctx, nil); err != nil {
			return err
		}

		// Ообновляем статус заказа в индексе.
		if err := workflow.ExecuteActivity(ctx, p.search.UpdateOrder, p.order.id, &elasticsearch.Order{Status: p.order.status}, true).Get(ctx, nil); err != nil {
			return err
		}

		// Уведомляем клиент пользователя, что статус заказа изменился.
		if err := workflow.ExecuteActivity(ctx, p.sseService.SendNotification,
			sse.NewOrderListUpdatedEvent().ForUser().WithID(p.order.user.id)).Get(ctx, nil); err != nil {

			return err
		}

		break
	}

	return nil
}

func (p *orderProcessing) giveAway(ctx workflow.Context) error {
	receiveSignals := workflow.GetSignalChannel(ctx, "receive_signals")
	receiveSelector := workflow.NewSelector(ctx)

	receiveSelector.AddReceive(receiveSignals, func(ch workflow.ReceiveChannel, more bool) {
		var rc ReceiveSignal
		ch.Receive(ctx, &rc)

		if p.order.pinCode != rc.PINCode {
			return
		}

		p.order.status = orderStatusReceived
	})

	for {
		receiveSelector.Select(ctx)

		if p.order.status == orderStatusReceived {
			break
		}

		var (
			logID uuid.UUID
			text  = fmt.Sprintf("Неправильный пинкод, попробуйте ещё раз")
		)

		// Создаём новый идентификатор для записи лога.
		if err := workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
			return uuid.New()
		}).Get(&logID); err != nil {
			return err
		}

		p.order.logs = append(p.order.logs, &LogItem{
			id:   logID,
			Text: text,
		})

		// Логируем, что была неудачная попытка ввести пинкод.
		if err := workflow.ExecuteActivity(ctx, p.storage.LogAttemptToEnterWrongPINCode, postgres.LogAttemptToEnterWrongPINCodeParams{
			ID:      logID,
			Reason:  text,
			OrderID: p.order.id,
		}).Get(ctx, nil); err != nil {
			return err
		}

		// Обновляем логи в индексе.

		logs := make([]*elasticsearch.LogItem, 0)
		for _, l := range p.order.logs {
			logs = append(logs, &elasticsearch.LogItem{
				ID:      l.id.String(),
				Text:    l.Text,
				OrderID: p.order.id.String(),
			})
		}

		if err := workflow.ExecuteActivity(ctx, p.search.UpdateOrder, p.order.id, &elasticsearch.Order{LogItems: logs}, true).Get(ctx, nil); err != nil {
			return err
		}

		// Отправляем уведомление.
		if err := workflow.ExecuteActivity(ctx, p.sseService.SendNotification,
			sse.NewAttemptToEnterWrongPINCodeEvent().ForUser().WithID(p.order.user.id)).Get(ctx, nil); err != nil {

			return err
		}
	}

	return nil
}

func (p *orderProcessing) cleanUp(ctx workflow.Context) error {
	// Убираем заказ из списка закакоз для кассы.
	if err := workflow.ExecuteActivity(ctx, p.storage.RemoveCacheOrderAsReady, p.order.id).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем кассу, что бы обновила список заказов.
	if err := workflow.ExecuteActivity(ctx, p.sseService.SendNotification,
		sse.NewOrderListUpdatedEvent().ForCache().WithID(p.order.point.cacheID)).Get(ctx, nil); err != nil {

		return err
	}

	// Обновляем статус заказа для клиента пользователя.
	if err := workflow.ExecuteActivity(ctx, p.storage.UpdateOrderStatus, p.order.id, p.order.status).Get(ctx, nil); err != nil {
		return err
	}

	// Ообновляем статус заказа в индексе.
	if err := workflow.ExecuteActivity(ctx, p.search.UpdateOrder, p.order.id, &elasticsearch.Order{Status: p.order.status}, true).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем клиент пользователя, что статус заказа изменился.
	if err := workflow.ExecuteActivity(ctx, p.sseService.SendNotification,
		sse.NewOrderListUpdatedEvent().ForUser().WithID(p.order.user.id)).Get(ctx, nil); err != nil {

		return err
	}

	return nil
}
