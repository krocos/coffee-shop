package main

import (
	"fmt"

	"github.com/maxence-charriere/go-app/v9/pkg/app"
)

type KitchenSelector struct {
	app.Compo
	Points []*Point
}

func (s *KitchenSelector) Render() app.UI {
	return app.Div().Class("dropdown").Body(
		app.Button().Class("btn btn-primary dropdown-toggle").
			Type("button").Attr("data-bs-toggle", "dropdown").
			Text("Выбери кухню"),
		app.Ul().Class("dropdown-menu").Body(
			app.Range(s.Points).Slice(func(i int) app.UI {
				return app.Li().Body(
					app.A().Href("#").Class("dropdown-item").
						Text(fmt.Sprintf("Кухня на улице %s", s.Points[i].Addr)).
						OnClick(s.selectKitchen(s.Points[i])),
				)
			}),
		),
	)
}

func (s *KitchenSelector) selectKitchen(point *Point) app.EventHandler {
	return func(ctx app.Context, e app.Event) {
		ctx.NewActionWithValue("kitchenSelected", point)
	}
}
