package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v9/pkg/app"
)

type OrderMaker struct {
	app.Compo

	userID uuid.UUID
	items  []*Item
	points []*Point

	selectedItems   map[string]*selectedItemState
	totalPrice      float64
	selectedPointID uuid.UUID
}

type selectedItemState struct {
	num   int
	total float64
}

func NewUserOrderMaker(userID uuid.UUID, items []*Item, points []*Point) *OrderMaker {
	m := &OrderMaker{
		userID:        userID,
		items:         items,
		points:        points,
		selectedItems: make(map[string]*selectedItemState),
	}

	for _, item := range items {
		m.selectedItems[item.ID.String()] = new(selectedItemState)
	}

	return m
}

func (m *OrderMaker) Render() app.UI {
	return app.Div().Class("row").Body(
		app.Div().Class("col").Body(
			app.Div().Class("card").Body(
				app.Div().Class("card-body").Body(
					app.Range(m.items).Slice(func(i int) app.UI {
						return app.Div().Class("row").Body(
							app.Div().Class("col-4", "col-sm-4", "col-md-2", "col-lg-2", "text-end").Body(
								app.P().Text(fmt.Sprintf("%.2f₽", m.items[i].Price)),
							),
							app.Div().Class("col-5", "col-sm-5", "col-md-4", "col-lg-4").Body(
								app.P().Text(m.items[i].Title),
							),
							app.Div().Class("col-3", "col-sm-3", "col-md-2", "col-lg-2", "text-end").Body(
								app.Div().Class("btn-group").Attr("role", "group").Body(
									app.Button().Type("button").Value(m.items[i].ID.String()).
										Class("btn", "btn-warning", "btn-sm").Text("▼").OnClick(m.decreaseItemNum),
									app.Button().Type("button").Value(m.items[i].ID.String()).
										Class("btn", "btn-success", "btn-sm").Text("▲").OnClick(m.increaseItemNum),
								),
							),
							app.Div().Class("col-4", "col-sm-4", "col-md-2", "col-lg-2", "text-end").Body(
								app.P().Text(fmt.Sprintf("%d", m.selectedItems[m.items[i].ID.String()].num)),
							),
							app.Div().Class("col-8", "col-sm-8", "col-md-2", "col-lg-2", "text-end").Body(
								app.P().Text(fmt.Sprintf("%.2f₽", m.selectedItems[m.items[i].ID.String()].total)),
							),
						)
					}),
					app.If(m.totalPrice > 0,
						app.Div().Class("row").Body(
							app.Div().Class("col").Body(
								app.Br(),
								app.H3().Text("Новый заказ"),
								app.Hr(),
							),
						),
						app.Div().Class("row").Body(
							app.Div().Class("col").Body(
								app.H4().Text("Итого"),
							),
							app.Div().Class("col", "text-end").Body(
								app.H4().Text(fmt.Sprintf("%.2f₽", m.totalPrice)),
							),
						),
						app.Div().Class("row").Body(
							app.Div().Class("col").Body(
								app.Select().Class("form-select").Body(
									app.Option().Disabled(true).Selected(true).Text("Где готовить?"),
									app.Range(m.points).Slice(func(i int) app.UI {
										return app.Option().Value(m.points[i].ID.String()).Text(m.points[i].Addr)
									}),
								).OnChange(m.selectPoint),
							),
						),
						app.Div().Class("row").Body(
							app.Div().Class("col", "text-end").Body(
								app.Hr(),
								app.Button().Type("button").Class("btn", "btn-light", "btn-sm").Text("Сбросить").OnClick(m.clearSelectedHandler),
								app.Span().Text(" "),
								app.If(m.selectedPointID == uuid.Nil,
									app.Button().Type("button").Class("btn", "btn-primary").Disabled(true).Text("Заказать"),
								).Else(
									app.Button().Type("button").Class("btn", "btn-primary").Text("Заказать").OnClick(m.createNewOrder),
								),
							),
						),
					),
				),
			),
		),
	)
}

type (
	CreateOrderRequest struct {
		UserID  uuid.UUID                 `json:"user_id"`
		PointID uuid.UUID                 `json:"point_id"`
		Items   []*InitialItemDataRequest `json:"items"`
	}
	InitialItemDataRequest struct {
		ID       uuid.UUID `json:"id"`
		Quantity float64   `json:"quantity"`
	}
)

func (m *OrderMaker) createNewOrder(ctx app.Context, e app.Event) {
	req := &CreateOrderRequest{
		UserID:  m.userID,
		PointID: m.selectedPointID,
		Items:   make([]*InitialItemDataRequest, 0),
	}

	for id, state := range m.selectedItems {
		if state.num > 0 {
			req.Items = append(req.Items, &InitialItemDataRequest{
				ID:       uuid.MustParse(id),
				Quantity: float64(state.num),
			})
		}
	}

	m.clearSelected()

	ctx.Async(func() {
		bb, err := json.Marshal(req)
		if err != nil {
			app.Log(err)
			return
		}

		res, err := http.Post(fmt.Sprintf("http://%s:8888/user-api/order", host), "application/json", bytes.NewReader(bb))
		if err != nil {
			app.Log(err)
			return
		}
		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusCreated {
			bb, err := io.ReadAll(res.Body)
			if err != nil {
				app.Log(fmt.Sprintf("bad response '%d %s'",
					res.StatusCode, http.StatusText(res.StatusCode)))
				return
			}

			app.Log(fmt.Sprintf("bad response '%d %s': %s",
				res.StatusCode, http.StatusText(res.StatusCode), string(bb)))
			return
		}
	})
}

func (m *OrderMaker) selectPoint(ctx app.Context, e app.Event) {
	m.selectedPointID = uuid.MustParse(e.JSValue().Get("target").Get("value").String())
}

func (m *OrderMaker) clearSelectedHandler(ctx app.Context, e app.Event) {
	m.clearSelected()
}

func (m *OrderMaker) clearSelected() {
	for _, state := range m.selectedItems {
		state.num = 0
		state.total = 0.0
	}
	m.totalPrice = 0.0
	m.selectedPointID = uuid.Nil
}

func (m *OrderMaker) itemPrice(itemID uuid.UUID) float64 {
	for _, item := range m.items {
		if item.ID.String() == itemID.String() {
			return item.Price
		}
	}
	return 0.0
}

func (m *OrderMaker) updateTotalPrice() {
	var total = 0.0
	for _, state := range m.selectedItems {
		total += state.total
	}
	m.totalPrice = total
}

func (m *OrderMaker) increaseItemNum(ctx app.Context, e app.Event) {
	itemID := uuid.MustParse(e.JSValue().Get("target").Get("value").String())
	state := m.selectedItems[itemID.String()]
	state.num++
	if state.num > 5 {
		state.num = 5
	}
	state.total = float64(state.num) * m.itemPrice(itemID)
	m.updateTotalPrice()
}

func (m *OrderMaker) decreaseItemNum(ctx app.Context, e app.Event) {
	itemID := uuid.MustParse(e.JSValue().Get("target").Get("value").String())
	state := m.selectedItems[itemID.String()]
	state.num--
	if state.num < 0 {
		state.num = 0
	}
	state.total = float64(state.num) * m.itemPrice(itemID)
	m.updateTotalPrice()
}
