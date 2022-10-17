package main

import (
	"fmt"

	"github.com/maxence-charriere/go-app/v9/pkg/app"
)

type CacheSelector struct {
	app.Compo
	Points []*Point
}

func (s *CacheSelector) Render() app.UI {
	return app.Div().Class("dropdown").Body(
		app.Button().Class("btn btn-primary dropdown-toggle").
			Type("button").Attr("data-bs-toggle", "dropdown").
			Text("Выбери кассу"),
		app.Ul().Class("dropdown-menu").Body(
			app.Range(s.Points).Slice(func(i int) app.UI {
				return app.Li().Body(
					app.A().Href("#").Class("dropdown-item").
						Text(fmt.Sprintf("Касса на улице %s", s.Points[i].Addr)).
						OnClick(s.selectCache(s.Points[i])),
				)
			}),
		),
	)
}

func (s *CacheSelector) selectCache(point *Point) app.EventHandler {
	return func(ctx app.Context, e app.Event) {
		ctx.NewActionWithValue("cacheSelected", point)
	}
}
