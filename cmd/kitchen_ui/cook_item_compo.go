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

type CookItemCompo struct {
	app.Compo
	CookItem *KitchenCookItemResponse
}

func (c *CookItemCompo) Render() app.UI {
	return app.Div().Class("row").Body(
		app.Div().Class("col").Body(
			app.Div().Class("card").Body(
				app.Div().Class("card-body").Body(
					app.Div().Class("row").Body(
						app.Div().Class("col-7").Body(
							app.Span().Class("card-title").Style("font-size", "1.3em").
								Text(c.CookItem.Title),
						),
						app.Div().Class("col", "text-end").Body(
							app.Span().Class("card-title").Style("font-size", "1.3em").
								Text(fmt.Sprintf("%.0f", c.CookItem.Quantity)),
						),
						app.Div().Class("col", "text-end").Body(
							app.Button().Type("button").Class("btn", "btn-primary", "btn-sm").
								Text("Готово").OnClick(c.cookItemReady),
						),
					),
					app.Div().Class("row").Body(
						app.Div().Class("col").Body(
							app.Small().Class("text-muted").Text(c.CookItem.ID.String()),
						),
					),
				),
			),
			app.Br(),
		),
	)
}

type OrderItemCookedRequest struct {
	OrderItemID uuid.UUID `json:"order_item_id"`
}

func (c *CookItemCompo) cookItemReady(ctx app.Context, e app.Event) {
	bb, err := json.Marshal(&OrderItemCookedRequest{
		OrderItemID: c.CookItem.ID,
	})
	if err != nil {
		app.Log(err)
		return
	}

	res, err := http.Post(fmt.Sprintf("http://%s/kitchen-api/order/%s/item-cooked", host, c.CookItem.OrderID.String()),
		"application/json", bytes.NewReader(bb))
	if err != nil {
		app.Log(err)
		return
	}

	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		bb, err := io.ReadAll(res.Body)
		if err != nil {
			app.Log(fmt.Errorf("bad response '%d %s'", res.StatusCode, http.StatusText(res.StatusCode)))
			return
		}

		app.Log(fmt.Errorf("bad response '%d %s': %s", res.StatusCode, http.StatusText(res.StatusCode), string(bb)))
	}
}
