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
	loc, _ := time.LoadLocation("Asia/Yekaterinburg")

	var (
		storage    *postgres.Postgres
		sseService *sse.SSE
		search     *elasticsearch.Search

		order = &Order{
			id:        initialData.ID,
			createdAt: workflow.Now(ctx).In(loc),
		}
	)

	ao := workflow.ActivityOptions{StartToCloseTimeout: time.Hour}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Получаем данные пользователя. Тут это может пригодиться, что бы например вместе с этими данными получить
	// ещё и данные для предоставления скидки, например. В данном примере нас интересует только имя пользователя.
	var userData postgres.UserData
	if err := workflow.ExecuteActivity(ctx, storage.GetUserData, initialData.UserID).Get(ctx, &userData); err != nil {
		return err
	}

	order.user = &User{
		id:   userData.ID,
		name: userData.Name,
	}

	// Получаем данные итемов заказа, что бы можно было рассчитать общую стоимость заказа. Если бы мы имели
	// скидоную систему, то можно было бы после загрузки данных пользователя добавить и скидку, например.
	itemIDs := lo.Map(initialData.Items, func(item ItemInitialData, _ int) uuid.UUID { return item.ID })
	var itemsData []postgres.ItemData
	if err := workflow.ExecuteActivity(ctx, storage.GetItemsData, itemIDs).Get(ctx, &itemsData); err != nil {
		return err
	}

	order.orderItems = make([]*OrderItem, 0)
	for _, data := range itemsData {
		var orderItemID uuid.UUID

		// Создаём новый идентификатор товара в заказе.
		if err := workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} { return uuid.New() }).Get(&orderItemID); err != nil {
			return err
		}

		quantity := initialData.itemQuantity(data.ID)

		order.orderItems = append(order.orderItems, &OrderItem{
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
	if err := workflow.ExecuteActivity(ctx, storage.GetPointData, initialData.PointID).Get(ctx, &pointData); err != nil {
		return err
	}

	order.point = &Point{
		id:        pointData.ID,
		addr:      pointData.Addr,
		kitchenID: pointData.KitchenID,
		cacheID:   pointData.CacheID,
	}

	// Создаём пинкод для выдачи заказа.
	if err := workflow.SideEffect(ctx, func(ctx workflow.Context) interface{} {
		return fmt.Sprintf("%04d", rand.Intn(10000))
	}).Get(&order.pinCode); err != nil {
		return err
	}

	// Назначаем оредеру статус, что ожидает оплаты.
	order.status = orderStatusWaitingForPayment

	// Считаем общую сумму заказа.
	for _, item := range order.orderItems {
		order.totalPrice += item.totalPrice
	}

	// Записываем ордер в базу для длинной истории.
	createOrderParams := postgres.OrderParams{
		ID:         order.id,
		CreatedAt:  order.createdAt,
		Status:     order.status,
		TotalPrice: order.totalPrice,
		PINCode:    order.pinCode,
		UserID:     order.user.id,
		PointID:    order.point.id,
		Items:      make([]postgres.OrderItemParams, 0),
	}
	for _, orderItem := range order.orderItems {
		createOrderParams.Items = append(createOrderParams.Items, postgres.OrderItemParams{
			ID:         orderItem.id,
			Title:      orderItem.title,
			Price:      orderItem.price,
			ItemID:     orderItem.itemID,
			Quantity:   orderItem.quantity,
			TotalPrice: orderItem.totalPrice,
		})
	}
	if err := workflow.ExecuteActivity(ctx, storage.CreateOrder, createOrderParams).Get(ctx, nil); err != nil {
		return err
	}

	// Индексируем ордер для быстрой выдачи на клиент пользователя (истоиря ордеров бесконечно пополняется).

	searchOrder := elasticsearch.Order{
		ID:         order.id.String(),
		CreatedAt:  order.createdAt.Format(time.RFC3339),
		Status:     order.status,
		TotalPrice: order.totalPrice,
		PINCode:    order.pinCode,
		User: &elasticsearch.User{
			ID:   order.user.id.String(),
			Name: order.user.name,
		},
		Point: &elasticsearch.Point{
			ID:        order.point.id.String(),
			Addr:      order.point.addr,
			KitchenID: order.point.kitchenID.String(),
			CacheID:   order.point.cacheID.String(),
		},
	}

	for _, item := range order.orderItems {
		searchOrder.Items = append(searchOrder.Items, &elasticsearch.OrderItem{
			ID:         item.id.String(),
			Title:      item.title,
			Price:      item.price,
			ItemID:     item.itemID.String(),
			Quantity:   item.quantity,
			TotalPrice: item.totalPrice,
			OrderID:    order.id.String(),
		})
	}

	for _, logItem := range order.logs {
		searchOrder.LogItems = append(searchOrder.LogItems, &elasticsearch.LogItem{
			ID:      logItem.id.String(),
			Text:    logItem.Text,
			OrderID: order.id.String(),
		})
	}

	if err := workflow.ExecuteActivity(ctx, search.IndexOrder, order.id, searchOrder, true).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем клиента пользователя, что заказ создан и ожидает оплаты.
	if err := workflow.ExecuteActivity(ctx, sseService.SendNotification,
		sse.NewOrderListUpdatedEvent().ForUser().WithID(order.user.id)).Get(ctx, nil); err != nil {

		return err
	}

	// Ожидаем сигнала об оплате от платёжного интегратора, таймаута или отмены заказа.

	paymentSignals := workflow.GetSignalChannel(ctx, "payment_signals")

	for {
		if order.status != orderStatusWaitingForPayment {
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
				order.status = orderStatusPaid
			case paymentSignalUnsuccessful:
				unsuccessfulPaymentReason = s.Reason
			case paymentSignalCanceled:
				order.status = orderStatusPaymentCanceled
			}
		})
		paymentSelector.AddFuture(paymentTimeout, func(f workflow.Future) {
			// Устанавливаем статус, что оплата просрочена (заказ отменяется и выходим после селекта).
			order.status = orderStatusPaymentTimeout
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

			order.logs = append(order.logs, &LogItem{
				id:   logID,
				Text: text,
			})

			// Добавляем запись лога в базу данных.
			if err := workflow.ExecuteActivity(ctx, storage.LogUnsuccessfulPayment, postgres.LogUnsuccessfulPaymentParams{
				ID:      logID,
				OrderID: order.id,
				Reason:  text,
			}).Get(ctx, nil); err != nil {
				return err
			}

			// Обновляем логи в индексе.

			logs := make([]*elasticsearch.LogItem, 0)
			for _, l := range order.logs {
				logs = append(logs, &elasticsearch.LogItem{
					ID:      l.id.String(),
					Text:    l.Text,
					OrderID: order.id.String(),
				})
			}

			if err := workflow.ExecuteActivity(ctx, search.UpdateOrder, order.id, &elasticsearch.Order{LogItems: logs}, true).Get(ctx, nil); err != nil {
				return err
			}

			// Уведомляем пользователя о новых логах.
			if err := workflow.ExecuteActivity(ctx, sseService.SendNotification,
				sse.NewUnsuccessfulPayAttemptEvent().ForUser().WithID(order.user.id)).Get(ctx, nil); err != nil {

				return err
			}

			unsuccessfulPaymentReason = ""
		}
	}

	// Записываем измеение статуса ордера в базу данных для клинета пользователя.
	if err := workflow.ExecuteActivity(ctx, storage.UpdateOrderStatus, order.id, order.status).Get(ctx, nil); err != nil {
		return err
	}

	// Ообновляем статус заказа в индексе.
	if err := workflow.ExecuteActivity(ctx, search.UpdateOrder, order.id, &elasticsearch.Order{Status: order.status}, true).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем клиента пользователя, что статус заказа изменился.
	if err := workflow.ExecuteActivity(ctx, sseService.SendNotification,
		sse.NewOrderListUpdatedEvent().ForUser().WithID(order.user.id)).Get(ctx, nil); err != nil {

		return err
	}

	if order.status != orderStatusPaid {
		// Тут выходим ибо заказ не оплачен.
		return nil
	}

	// Записываем в заказы кухни на точке, какие итемы надо приготовить.
	cookingParams := postgres.AddItemsForCookingParams{
		KitchenID: order.point.kitchenID,
		OrderID:   order.id,
		Items:     make([]postgres.ItemForCooking, 0),
	}
	for _, item := range order.orderItems {
		cookingParams.Items = append(cookingParams.Items, postgres.ItemForCooking{
			ID:       item.id,
			Title:    item.title,
			Quantity: item.quantity,
		})
	}
	if err := workflow.ExecuteActivity(ctx, storage.AddItemsForKitchen, cookingParams).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем клиент кухни точки, что появились новые итемы на приготовку.
	if err := workflow.ExecuteActivity(ctx, sseService.SendNotification,
		sse.NewItemListUpdatedEvent().ForKitchen().WithID(order.point.kitchenID)).Get(ctx, nil); err != nil {

		return err
	}

	// Записываем в заказах кассы точки, что появился новый ордер и он отдан на кухню точки, что бы
	// оператор кассы знал что заказ готовится, что бы говорить с клиентом, если он рано явился.

	checkList := make([]string, 0)
	for _, item := range order.orderItems {
		checkList = append(checkList, fmt.Sprintf("%s %.0f шт.", item.title, item.quantity))
	}

	if err := workflow.ExecuteActivity(ctx, storage.AddNewOrderForCache, postgres.AddNewOrderForCacheParams{
		ID:               order.id,
		CacheID:          order.point.cacheID,
		OrderID:          order.id,
		UserName:         order.user.name,
		Status:           cacheOrderStatusCooking,
		ReadinessPercent: 0,
		CheckList:        strings.Join(checkList, ", "),
	}).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем клиент кассы точки, что есть готовящийся заказ.
	if err := workflow.ExecuteActivity(ctx, sseService.SendNotification,
		sse.NewOrderListUpdatedEvent().ForCache().WithID(order.point.cacheID)).Get(ctx, nil); err != nil {

		return err
	}

	order.status = orderStatusCooking

	// Изменить статус заказа для клиента пользователя, что заказ готовится.
	if err := workflow.ExecuteActivity(ctx, storage.UpdateOrderStatus, order.id, order.status).Get(ctx, nil); err != nil {
		return err
	}

	// Ообновляем статус заказа в индексе.
	if err := workflow.ExecuteActivity(ctx, search.UpdateOrder, order.id, &elasticsearch.Order{Status: order.status}, true).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомить клиент пользователя, что изменился статус заказа.
	if err := workflow.ExecuteActivity(ctx, sseService.SendNotification,
		sse.NewOrderListUpdatedEvent().ForUser().WithID(order.user.id)).Get(ctx, nil); err != nil {

		return err
	}

	// Начинаем слушать сигналы от кухни, что итемы приготовились.

	cookingSignals := workflow.GetSignalChannel(ctx, "cooking_signals")

	for {
		// Тут сделано через селектор, хотя, можно было бы просто слушать
		// канал игналов, но так будет проще потом улучшить петлю сигналов.

		cookingSelector := workflow.NewSelector(ctx)

		var cookedOrderItemID uuid.UUID

		cookingSelector.AddReceive(cookingSignals, func(ch workflow.ReceiveChannel, more bool) {
			var s CookingSignal
			ch.Receive(ctx, &s)

			for _, orderItem := range order.orderItems {
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

		for _, orderItem := range order.orderItems {
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
		if err := workflow.ExecuteActivity(ctx, storage.RemoveKitchenCookItemAsReady, cookedOrderItemID).Get(ctx, nil); err != nil {
			return err
		}

		// Уведомляем клиент кухни, что бы перезагузил заказы.
		if err := workflow.ExecuteActivity(ctx, sseService.SendNotification,
			sse.NewItemListUpdatedEvent().ForKitchen().WithID(order.point.kitchenID)).Get(ctx, nil); err != nil {

			return err
		}

		// Обновляем процент готовности на кассе.
		if err := workflow.ExecuteActivity(ctx, storage.UpdateCacheOrderReadinessPercent, order.id, readyPercent).Get(ctx, nil); err != nil {
			return err
		}

		if readyPercent == 100 {
			if err := workflow.ExecuteActivity(ctx, storage.UpdateCacheOrderStatus, order.id, cacheOrderStatusReady).Get(ctx, nil); err != nil {
				return err
			}
		}

		// Уведомляем клиент кассы, что изменился процент готовности заказа.
		if err := workflow.ExecuteActivity(ctx, sseService.SendNotification,
			sse.NewOrderListUpdatedEvent().ForCache().WithID(order.point.cacheID)).Get(ctx, nil); err != nil {

			return err
		}

		if readyPercent != 100 {
			continue
		}

		order.status = orderStatusReady

		// Если заказ готов, то обновляем статус заказа для клиента пользователя.
		if err := workflow.ExecuteActivity(ctx, storage.UpdateOrderStatus, order.id, order.status).Get(ctx, nil); err != nil {
			return err
		}

		// Ообновляем статус заказа в индексе.
		if err := workflow.ExecuteActivity(ctx, search.UpdateOrder, order.id, &elasticsearch.Order{Status: order.status}, true).Get(ctx, nil); err != nil {
			return err
		}

		// Уведомляем клиент пользователя, что статус заказа изменился.
		if err := workflow.ExecuteActivity(ctx, sseService.SendNotification,
			sse.NewOrderListUpdatedEvent().ForUser().WithID(order.user.id)).Get(ctx, nil); err != nil {

			return err
		}

		break
	}

	receiveSignals := workflow.GetSignalChannel(ctx, "receive_signals")
	receiveSelector := workflow.NewSelector(ctx)

	receiveSelector.AddReceive(receiveSignals, func(ch workflow.ReceiveChannel, more bool) {
		var rc ReceiveSignal
		ch.Receive(ctx, &rc)

		if order.pinCode != rc.PINCode {
			return
		}

		order.status = orderStatusReceived
	})

	for {
		receiveSelector.Select(ctx)

		if order.status == orderStatusReceived {
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

		order.logs = append(order.logs, &LogItem{
			id:   logID,
			Text: text,
		})

		// Логируем, что была неудачная попытка ввести пинкод.
		if err := workflow.ExecuteActivity(ctx, storage.LogAttemptToEnterWrongPINCode, postgres.LogAttemptToEnterWrongPINCodeParams{
			ID:      logID,
			Reason:  text,
			OrderID: order.id,
		}).Get(ctx, nil); err != nil {
			return err
		}

		// Обновляем логи в индексе.

		logs := make([]*elasticsearch.LogItem, 0)
		for _, l := range order.logs {
			logs = append(logs, &elasticsearch.LogItem{
				ID:      l.id.String(),
				Text:    l.Text,
				OrderID: order.id.String(),
			})
		}

		if err := workflow.ExecuteActivity(ctx, search.UpdateOrder, order.id, &elasticsearch.Order{LogItems: logs}, true).Get(ctx, nil); err != nil {
			return err
		}

		// Отправляем уведомление.
		if err := workflow.ExecuteActivity(ctx, sseService.SendNotification,
			sse.NewAttemptToEnterWrongPINCodeEvent().ForUser().WithID(order.user.id)).Get(ctx, nil); err != nil {

			return err
		}
	}

	// Тут мы уже считаем, что заказ был выдан.

	// Убираем заказ из списка закакоз для кассы.
	if err := workflow.ExecuteActivity(ctx, storage.RemoveCacheOrderAsReady, order.id).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем кассу, что бы обновила список заказов.
	if err := workflow.ExecuteActivity(ctx, sseService.SendNotification,
		sse.NewOrderListUpdatedEvent().ForCache().WithID(order.point.cacheID)).Get(ctx, nil); err != nil {

		return err
	}

	// Обновляем статус заказа для клиента пользователя.
	if err := workflow.ExecuteActivity(ctx, storage.UpdateOrderStatus, order.id, order.status).Get(ctx, nil); err != nil {
		return err
	}

	// Ообновляем статус заказа в индексе.
	if err := workflow.ExecuteActivity(ctx, search.UpdateOrder, order.id, &elasticsearch.Order{Status: order.status}, true).Get(ctx, nil); err != nil {
		return err
	}

	// Уведомляем клиент пользователя, что статус заказа изменился.
	if err := workflow.ExecuteActivity(ctx, sseService.SendNotification,
		sse.NewOrderListUpdatedEvent().ForUser().WithID(order.user.id)).Get(ctx, nil); err != nil {

		return err
	}

	return nil
}
