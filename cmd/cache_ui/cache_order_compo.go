package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/maxence-charriere/go-app/v9/pkg/app"
)

type CacheOrderCompo struct {
	app.Compo
	CacheOrder *CacheOrderResponse
	pinCode    string
}

var pinCodeRegex = regexp.MustCompile(`^\d{4}$`)

func (c *CacheOrderCompo) Render() app.UI {
	app.Log(c.CacheOrder)

	return app.Div().Class("row").Body(
		app.Div().Class("col").Body(
			app.Div().Class("card").Body(
				app.Div().Class("card-body").Body(
					app.Div().Class("row").Body(
						app.Div().Class("col").Body(
							app.Span().Class("card-title").Style("font-size", "1.3em").
								Text(c.CacheOrder.UserName),
						),
						app.If(c.CacheOrder.Status == "ready",
							app.If(pinCodeRegex.MatchString(c.pinCode),
								app.Div().Class("col-2").Body(
									app.Input().Type("text").Class("form-control", "form-control-sm").
										Attr("placeholder", "ПИН").OnInput(c.ValueTo(&c.pinCode)),
								),
								app.Div().Class("col-2", "text-end").Body(
									app.Button().Type("button").Class("btn", "btn-primary", "btn-sm").
										Text("Отдать").OnClick(c.receiveCacheOrder),
								),
							).Else(
								app.Div().Class("col-2").Body(
									app.Input().Type("text").Class("form-control", "form-control-sm", "is-invalid").
										Attr("placeholder", "ПИН").OnInput(c.ValueTo(&c.pinCode)),
								),
								app.Div().Class("col-2", "text-end").Body(
									app.Button().Type("button").Class("btn", "btn-primary", "btn-sm").
										Disabled(true).Text("Отдать"),
								),
							),
						),
					),
					app.Div().Class("row").Body(
						app.Div().Class("col").Body(
							app.Small().Class("text-muted").Text(c.CacheOrder.ID.String()),
						),
					),
					app.Div().Class("row").Body(
						app.Div().Class("col").Body(
							app.P().Text(c.CacheOrder.CheckList),
						),
					),
					app.If(c.CacheOrder.Status == "cooking",
						app.Div().Class("progress").Style("height", "3px").Body(
							app.Div().
								Class("progress-bar").
								Style("width", fmt.Sprintf("%d%%", c.CacheOrder.ReadinessPercent)).
								Attr("role", "").
								Attr("aria-valuenow", fmt.Sprintf("%d", c.CacheOrder.ReadinessPercent)).
								Attr("aria-valuemin", "0").
								Attr("aria-valuemax", "100"),
						),
					).Else(
						app.Div().Class("progress").Style("height", "3px").Body(
							app.Div().
								Class("progress-bar", "bg-success").
								Style("width", fmt.Sprintf("%d%%", c.CacheOrder.ReadinessPercent)).
								Attr("role", "").
								Attr("aria-valuenow", fmt.Sprintf("%d", c.CacheOrder.ReadinessPercent)).
								Attr("aria-valuemin", "0").
								Attr("aria-valuemax", "100"),
						),
					),
				),
			),
			app.Br(),
		),
	)
}

type ReceiveOrderRequest struct {
	PINCode string `json:"pin_code"`
}

func (c *CacheOrderCompo) receiveCacheOrder(ctx app.Context, e app.Event) {
	bb, err := json.Marshal(&ReceiveOrderRequest{
		PINCode: c.pinCode,
	})
	if err != nil {
		app.Log(err)
		return
	}

	res, err := http.Post(fmt.Sprintf("http://%s:8888/cache-api/order/%s/receive-order", host, c.CacheOrder.OrderID.String()),
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
