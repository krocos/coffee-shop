package handling

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.temporal.io/sdk/client"

	"github.com/krocos/coffee-shop/backend"
	"github.com/krocos/coffee-shop/postgres"
)

type Storage interface {
	GetMenu(ctx context.Context) (*postgres.MenuResponse, error)
	ListKitchenCookItems(ctx context.Context, kitchenID uuid.UUID) ([]*postgres.KitchenCookItemResponse, error)
	ListCacheOrders(ctx context.Context, cacheID uuid.UUID) ([]*postgres.CacheOrderResponse, error)
}

type Search interface {
	ListUserOrders(ctx context.Context, userID uuid.UUID) ([]json.RawMessage, error)
}

type Handling struct {
	client  client.Client
	storage Storage
	search  Search
}

func NewHandling(client client.Client, storage Storage, search Search) *Handling {
	return &Handling{
		client:  client,
		storage: storage,
		search:  search,
	}
}

type CreateOrderRequest struct {
	UserID  uuid.UUID                 `json:"user_id"`
	PointID uuid.UUID                 `json:"point_id"`
	Items   []*InitialItemDataRequest `json:"items"`
}
type InitialItemDataRequest struct {
	ID       uuid.UUID `json:"id"`
	Quantity float64   `json:"quantity"`
}

func (h *Handling) CreateOrder(w http.ResponseWriter, r *http.Request) {
	req := new(CreateOrderRequest)
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	orderID := uuid.New()

	initialData := backend.OrderInitialData{
		ID:      orderID,
		UserID:  req.UserID,
		PointID: req.PointID,
		Items:   make([]backend.ItemInitialData, 0),
	}
	for _, item := range req.Items {
		initialData.Items = append(initialData.Items, backend.ItemInitialData{
			ID:       item.ID,
			Quantity: item.Quantity,
		})
	}

	if _, err := h.client.ExecuteWorkflow(context.Background(), client.StartWorkflowOptions{
		ID:        orderWorkflowID(orderID),
		TaskQueue: "coffee",
	}, backend.OrderWorkflow, initialData); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")

	_ = json.NewEncoder(w).Encode(orderID)
}

type OrderItemCookedRequest struct {
	OrderItemID uuid.UUID `json:"order_item_id"`
}

func (h *Handling) OrderItemCooked(w http.ResponseWriter, r *http.Request) {
	orderID, err := uuid.Parse(mux.Vars(r)["order_id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req := new(OrderItemCookedRequest)
	if err = json.NewDecoder(r.Body).Decode(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err = h.client.SignalWorkflow(context.Background(), orderWorkflowID(orderID), "", "cooking_signals", backend.CookingSignal{
		OrderItemID: req.OrderItemID,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type ReceiveOrderRequest struct {
	PINCode string `json:"pin_code"`
}

func (h *Handling) ReceiveOrder(w http.ResponseWriter, r *http.Request) {
	orderID, err := uuid.Parse(mux.Vars(r)["order_id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req := new(ReceiveOrderRequest)
	if err = json.NewDecoder(r.Body).Decode(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err = h.client.SignalWorkflow(context.Background(), orderWorkflowID(orderID), "", "receive_signals", backend.ReceiveSignal{
		PINCode: req.PINCode,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type PaymentEventRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func (h *Handling) PaymentEvent(w http.ResponseWriter, r *http.Request) {
	orderID, err := uuid.Parse(mux.Vars(r)["order_id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req := new(PaymentEventRequest)
	if err = json.NewDecoder(r.Body).Decode(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err = h.client.SignalWorkflow(context.Background(), orderWorkflowID(orderID), "", "payment_signals", backend.PaymentSignal{
		Status: req.Status,
		Reason: req.Reason,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type User struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Item struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
	Price float64   `json:"price"`
}

type Point struct {
	ID        uuid.UUID `json:"id"`
	Addr      string    `json:"addr"`
	KitchenID uuid.UUID `json:"kitchen_id"`
	CacheID   uuid.UUID `json:"cache_id"`
}

type MenuResponse struct {
	Users  []*User  `json:"users"`
	Items  []*Item  `json:"items"`
	Points []*Point `json:"points"`
}

func (h *Handling) GetMenu(w http.ResponseWriter, r *http.Request) {
	menu, err := h.storage.GetMenu(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res := &MenuResponse{
		Users:  make([]*User, 0),
		Items:  make([]*Item, 0),
		Points: make([]*Point, 0),
	}

	for _, user := range menu.Users {
		res.Users = append(res.Users, (*User)(user))
	}

	for _, item := range menu.Items {
		res.Items = append(res.Items, (*Item)(item))
	}

	for _, point := range menu.Points {
		res.Points = append(res.Points, (*Point)(point))
	}

	w.Header().Set("Content-Type", "application/json")

	_ = json.NewEncoder(w).Encode(res)
}

func (h *Handling) ListUserOrders(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(mux.Vars(r)["user_id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rawOrders, err := h.search.ListUserOrders(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	_ = json.NewEncoder(w).Encode(rawOrders)
}

type KitchenCookItemResponse struct {
	ID       uuid.UUID `json:"id"`
	Title    string    `json:"title"`
	Quantity float64   `json:"quantity"`
	OrderID  uuid.UUID `json:"order_id"`
}

func (h *Handling) ListKitchenCookItems(w http.ResponseWriter, r *http.Request) {
	kitchenID, err := uuid.Parse(mux.Vars(r)["kitchen_id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cookItems, err := h.storage.ListKitchenCookItems(r.Context(), kitchenID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res := make([]*KitchenCookItemResponse, 0)

	for _, item := range cookItems {
		res = append(res, (*KitchenCookItemResponse)(item))
	}

	w.Header().Set("Content-Type", "application/json")

	_ = json.NewEncoder(w).Encode(res)
}

type CacheOrderResponse struct {
	ID               uuid.UUID `json:"id"`
	OrderID          uuid.UUID `json:"order_id"`
	UserName         string    `json:"user_name"`
	Status           string    `json:"status"`
	ReadinessPercent int       `json:"readiness_percent"`
	CheckList        string    `json:"check_list"`
}

func (h *Handling) ListCacheOrders(w http.ResponseWriter, r *http.Request) {
	cacheID, err := uuid.Parse(mux.Vars(r)["cache_id"])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cacheOrders, err := h.storage.ListCacheOrders(r.Context(), cacheID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	res := make([]*CacheOrderResponse, 0)

	for _, order := range cacheOrders {
		res = append(res, (*CacheOrderResponse)(order))
	}

	w.Header().Set("Content-Type", "application/json")

	_ = json.NewEncoder(w).Encode(res)
}

func orderWorkflowID(orderID uuid.UUID) string {
	return fmt.Sprintf("order:%s", orderID.String())
}
