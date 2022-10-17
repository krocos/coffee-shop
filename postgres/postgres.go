package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Postgres struct {
	db *gorm.DB
}

func NewPostgres(db *gorm.DB) *Postgres {
	return &Postgres{db: db}
}

type UserData struct {
	ID   uuid.UUID
	Name string
}

func (p *Postgres) GetUserData(ctx context.Context, userID uuid.UUID) (UserData, error) {
	user := new(User)
	if err := p.db.WithContext(ctx).Take(user, userID).Error; err != nil {
		return UserData{}, err
	}
	return UserData{
		ID:   user.ID,
		Name: user.Name,
	}, nil
}

type ItemData struct {
	ID    uuid.UUID
	Title string
	Price float64
}

func (p *Postgres) GetItemsData(ctx context.Context, itemIDs []uuid.UUID) ([]ItemData, error) {
	items := make([]*Item, 0)
	if err := p.db.WithContext(ctx).
		Order("title").
		Where("id in ?", itemIDs).
		Find(&items).Error; err != nil {
		return nil, err
	}

	list := make([]ItemData, 0)

	for _, item := range items {
		list = append(list, ItemData{
			ID:    item.ID,
			Title: item.Title,
			Price: item.Price,
		})
	}

	return list, nil
}

type PointData struct {
	ID        uuid.UUID
	Addr      string
	KitchenID uuid.UUID
	CacheID   uuid.UUID
}

func (p *Postgres) GetPointData(ctx context.Context, pointID uuid.UUID) (PointData, error) {
	point := new(Point)

	if err := p.db.WithContext(ctx).Take(point, pointID).Error; err != nil {
		return PointData{}, err
	}

	return PointData{
		ID:        point.ID,
		Addr:      point.Addr,
		KitchenID: point.KitchenID,
		CacheID:   point.CacheID,
	}, nil
}

type (
	OrderParams struct {
		ID         uuid.UUID
		CreatedAt  time.Time
		Status     string
		TotalPrice float64
		PINCode    string
		UserID     uuid.UUID
		PointID    uuid.UUID
		Items      []OrderItemParams
	}
	OrderItemParams struct {
		ID         uuid.UUID
		Title      string
		Price      float64
		ItemID     uuid.UUID
		Quantity   float64
		TotalPrice float64
	}
)

func (p *Postgres) CreateOrder(ctx context.Context, params OrderParams) error {
	order := &Order{
		ID:         params.ID,
		CreatedAt:  params.CreatedAt,
		Status:     params.Status,
		TotalPrice: params.TotalPrice,
		PINCode:    params.PINCode,
		UserID:     params.UserID,
		PointID:    params.PointID,
		Items:      make([]*OrderItem, 0),
	}

	for _, itemParams := range params.Items {
		order.Items = append(order.Items, &OrderItem{
			ID:         itemParams.ID,
			Title:      itemParams.Title,
			Price:      itemParams.Price,
			ItemID:     itemParams.ItemID,
			Quantity:   itemParams.Quantity,
			TotalPrice: itemParams.TotalPrice,
		})
	}

	return p.db.WithContext(ctx).Create(order).Error
}

func (p *Postgres) UpdateOrderStatus(ctx context.Context, orderID uuid.UUID, status string) error {
	return p.db.WithContext(ctx).
		Model(Order{}).
		Where("id = ?", orderID).
		Update("status", status).Error
}

type LogUnsuccessfulPaymentParams struct {
	ID      uuid.UUID
	OrderID uuid.UUID
	Reason  string
}

func (p *Postgres) LogUnsuccessfulPayment(ctx context.Context, pp LogUnsuccessfulPaymentParams) error {
	item := &LogItem{
		ID:      pp.ID,
		Text:    pp.Reason,
		OrderID: pp.OrderID,
	}
	return p.db.WithContext(ctx).Create(item).Error
}

type (
	ItemForCooking struct {
		ID       uuid.UUID
		Title    string
		Quantity float64
	}
	AddItemsForCookingParams struct {
		KitchenID uuid.UUID
		OrderID   uuid.UUID
		Items     []ItemForCooking
	}
)

func (p *Postgres) AddItemsForKitchen(ctx context.Context, params AddItemsForCookingParams) error {
	items := make([]*CookItem, 0)
	for _, item := range params.Items {
		items = append(items, &CookItem{
			ID:        item.ID,
			Title:     item.Title,
			Quantity:  item.Quantity,
			KitchenID: params.KitchenID,
			OrderID:   params.OrderID,
		})
	}

	return p.db.WithContext(ctx).Create(items).Error
}

type AddNewOrderForCacheParams struct {
	ID               uuid.UUID
	CacheID          uuid.UUID
	OrderID          uuid.UUID
	UserName         string
	Status           string
	ReadinessPercent int
	CheckList        string
}

func (p *Postgres) AddNewOrderForCache(ctx context.Context, params AddNewOrderForCacheParams) error {
	order := &CacheOrder{
		ID:               params.ID,
		CacheID:          params.CacheID,
		OrderID:          params.OrderID,
		UserName:         params.UserName,
		Status:           params.Status,
		ReadinessPercent: params.ReadinessPercent,
		CheckList:        params.CheckList,
	}

	return p.db.WithContext(ctx).Create(order).Error
}

func (p *Postgres) RemoveKitchenCookItemAsReady(ctx context.Context, orderItemID uuid.UUID) error {
	return p.db.WithContext(ctx).Unscoped().Where("id = ?", orderItemID).Delete(&CookItem{}).Error
}

func (p *Postgres) UpdateCacheOrderReadinessPercent(ctx context.Context, orderID uuid.UUID, readyPercent int) error {
	return p.db.WithContext(ctx).
		Model(CacheOrder{}).
		Where("id = ?", orderID).
		Update("readiness_percent", readyPercent).Error
}

func (p *Postgres) UpdateCacheOrderStatus(ctx context.Context, orderID uuid.UUID, status string) error {
	return p.db.WithContext(ctx).
		Model(CacheOrder{}).
		Where("id = ?", orderID).
		Update("status", status).Error
}

func (p *Postgres) RemoveCacheOrderAsReady(ctx context.Context, cacheOrderID uuid.UUID) error {
	return p.db.WithContext(ctx).Unscoped().Where("id = ?", cacheOrderID).Delete(&CacheOrder{}).Error
}

type LogAttemptToEnterWrongPINCodeParams struct {
	ID      uuid.UUID
	Reason  string
	OrderID uuid.UUID
}

func (p *Postgres) LogAttemptToEnterWrongPINCode(ctx context.Context, pp LogAttemptToEnterWrongPINCodeParams) error {
	item := &LogItem{
		ID:      pp.ID,
		Text:    pp.Reason,
		OrderID: pp.OrderID,
	}
	return p.db.WithContext(ctx).Create(item).Error
}

type UserResponse struct {
	ID   uuid.UUID
	Name string
}

type ItemResponse struct {
	ID    uuid.UUID
	Title string
	Price float64
}

type PointResponse struct {
	ID        uuid.UUID
	Addr      string
	KitchenID uuid.UUID
	CacheID   uuid.UUID
}

type MenuResponse struct {
	Users  []*UserResponse
	Items  []*ItemResponse
	Points []*PointResponse
}

func (p *Postgres) GetMenu(ctx context.Context) (*MenuResponse, error) {
	users := make([]*User, 0)
	if err := p.db.WithContext(ctx).Order("id").Find(&users).Error; err != nil {
		return nil, err
	}
	items := make([]*Item, 0)
	if err := p.db.WithContext(ctx).Order("id").Find(&items).Error; err != nil {
		return nil, err
	}
	points := make([]*Point, 0)
	if err := p.db.WithContext(ctx).Order("id").Find(&points).Error; err != nil {
		return nil, err
	}

	menu := &MenuResponse{
		Users:  make([]*UserResponse, 0),
		Items:  make([]*ItemResponse, 0),
		Points: make([]*PointResponse, 0),
	}

	for _, user := range users {
		menu.Users = append(menu.Users, &UserResponse{
			ID:   user.ID,
			Name: user.Name,
		})
	}

	for _, item := range items {
		menu.Items = append(menu.Items, &ItemResponse{
			ID:    item.ID,
			Title: item.Title,
			Price: item.Price,
		})
	}

	for _, point := range points {
		menu.Points = append(menu.Points, &PointResponse{
			ID:        point.ID,
			Addr:      point.Addr,
			KitchenID: point.KitchenID,
			CacheID:   point.CacheID,
		})
	}

	return menu, nil
}

type KitchenCookItemResponse struct {
	ID       uuid.UUID
	Title    string
	Quantity float64
	OrderID  uuid.UUID
}

func (p *Postgres) ListKitchenCookItems(ctx context.Context, kitchenID uuid.UUID) ([]*KitchenCookItemResponse, error) {
	cookItems := make([]*CookItem, 0)
	if err := p.db.WithContext(ctx).Order("created_at").
		Where("kitchen_id = ?", kitchenID).
		Find(&cookItems).Error; err != nil {

		return nil, err
	}

	res := make([]*KitchenCookItemResponse, 0)
	for _, cookItem := range cookItems {
		res = append(res, &KitchenCookItemResponse{
			ID:       cookItem.ID,
			Title:    cookItem.Title,
			Quantity: cookItem.Quantity,
			OrderID:  cookItem.OrderID,
		})
	}

	return res, nil
}

type CacheOrderResponse struct {
	ID               uuid.UUID
	OrderID          uuid.UUID
	UserName         string
	Status           string
	ReadinessPercent int
	CheckList        string
}

func (p *Postgres) ListCacheOrders(ctx context.Context, cacheID uuid.UUID) ([]*CacheOrderResponse, error) {
	cacheOrders := make([]*CacheOrder, 0)
	if err := p.db.WithContext(ctx).Order("created_at").
		Where("cache_id = ?", cacheID).
		Find(&cacheOrders).Error; err != nil {

		return nil, err
	}

	res := make([]*CacheOrderResponse, 0)
	for _, order := range cacheOrders {
		res = append(res, &CacheOrderResponse{
			ID:               order.ID,
			OrderID:          order.OrderID,
			UserName:         order.UserName,
			Status:           order.Status,
			ReadinessPercent: order.ReadinessPercent,
			CheckList:        order.CheckList,
		})
	}

	return res, nil
}
