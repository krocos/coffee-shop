package main

import (
	"fmt"

	"github.com/maxence-charriere/go-app/v9/pkg/app"
)

type ItemsListCompo struct {
	app.Compo
	Items []*UserOrderItemResponse
}

func (c *ItemsListCompo) Render() app.UI {
	return app.Div().Class("row").Body(
		app.Div().Class("col").Style("font-size", "0.8em").Body(
			app.Range(c.Items).Slice(func(i int) app.UI {
				return app.If(true,
					app.Div().Class("row").Body(
						app.Div().Class("col-4", "col-sm-4", "col-md-5", "col-lg-5").Text(c.Items[i].Title),
						app.Div().Class("col-4", "col-sm-4", "col-md-4", "col-lg-4", "text-end").
							Body(app.Small().Text(fmt.Sprintf("%.0f по %.2f₽", c.Items[i].Quantity, c.Items[i].Price))),
						app.Div().Class("col-4", "col-sm-4", "col-md-3", "col-lg-3", "text-end").
							Text(fmt.Sprintf("%.2f₽", c.Items[i].TotalPrice)),
					),
					app.Div().Class("row").Body(
						app.Div().Class("col").Style("font-size", "0.8em").Body(
							app.Small().Class("text-muted").
								Text(c.Items[i].ID.String())),
					),
				)
			}),
		),
	)
}
