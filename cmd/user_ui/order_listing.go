package main

import (
	"github.com/maxence-charriere/go-app/v9/pkg/app"
)

type OrderListing struct {
	app.Compo

	Orders []*UserOrderResponse
}

func (l *OrderListing) Render() app.UI {
	if len(l.Orders) == 0 {
		return app.P().Class("text-muted").Text("Нет активных заказов")
	}

	return app.Div().Class("row").Body(
		app.Div().Class("col").Body(
			app.Range(l.Orders).Slice(func(i int) app.UI {
				return &OrderCompo{Order: l.Orders[i]}
			}),
		),
	)
}
