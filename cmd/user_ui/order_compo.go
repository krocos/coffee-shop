package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"

	"github.com/maxence-charriere/go-app/v9/pkg/app"
)

type OrderCompo struct {
	app.Compo
	Order *UserOrderResponse
}

func (c *OrderCompo) Render() app.UI {
	return app.Div().Class("row").Body(
		app.Div().Class("col").Body(

			app.Div().Class("card").Body(
				app.Div().Class("card-body").Body(
					app.Div().Class("row").Body(
						app.Div().Class("col").Body(
							app.H5().Class("card-title").Text(c.Order.Point.Addr),
						),
						app.Div().Class("col", "text-end").Body(
							app.H5().Class("card-title").Text(fmt.Sprintf("%.2f₽", c.Order.TotalPrice)),
						),
					),
					app.Div().Class("row").Body(
						app.Div().Class("col", "col-8").Body(
							app.P().Class("card-text", "text-muted").Style("font-size", "0.9em").Text(c.Order.ID.String()),
						),
						app.Div().Class("col", "text-end").Body(
							statusText(c.Order.Status),
						),
					),
					app.If(c.Order.Status == "waiting_for_payment",
						app.Div().Class("row").Body(
							app.Div().Class("col", "text-end").Body(
								app.Br(),
								app.Button().Type("button").Class("btn btn-danger btn-sm").Text("Отменить").
									OnClick(c.cancelOrder),
								app.Span().Text(" "),
								app.Button().Type("button").Class("btn btn-warning btn-sm").Text("Неудача").
									OnClick(c.simulateUnsuccessfulPayment),
								app.Span().Text(" "),
								app.Button().Type("button").Class("btn btn-primary").Text("Оплатить").
									OnClick(c.payForOrder),
							),
						),
					),
					app.If(c.Order.Status == "ready",
						app.Hr(),
						app.H2().Text(fmt.Sprintf("PIN: %s", c.Order.PINCode)),
					),
					app.If(true,
						app.Hr(),
						app.Div().Class("row").Body(
							app.Div().Class("col").Body(
								app.Small().Text(c.Order.CreatedAt.Format("02.01.2006 15:04 MST")),
							),
						),
					),
					app.If(true,
						app.Hr(),
						&ItemsListCompo{Items: c.Order.Items},
					),
					app.If(len(c.Order.LogItems) > 0,
						app.Hr(),
						app.Div().Class("row").Body(
							app.Div().Class("col").Body(
								app.Range(c.Order.LogItems).Slice(func(i int) app.UI {
									return app.Div().Class("card-text").Style("font-size", "0.8em").
										Text(fmt.Sprintf("⚠️ %s", c.Order.LogItems[i].Text))
								}),
							),
						),
					),
				),
			),
			app.Br(),
		),
	)
}

func (c *OrderCompo) cancelOrder(ctx app.Context, e app.Event) {
	bb, err := json.Marshal(&PaymentEventRequest{
		Status: "canceled",
	})
	if err != nil {
		app.Log(err)
		return
	}

	res, err := http.Post(fmt.Sprintf("http://%s:8888/payment-gateway-api/order/%s/payment-event", host, c.Order.ID.String()),
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

func (c *OrderCompo) payForOrder(ctx app.Context, e app.Event) {
	bb, err := json.Marshal(&PaymentEventRequest{
		Status: "successful",
	})
	if err != nil {
		app.Log(err)
		return
	}

	res, err := http.Post(fmt.Sprintf("http://%s:8888/payment-gateway-api/order/%s/payment-event", host, c.Order.ID.String()),
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

type PaymentEventRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func (c *OrderCompo) simulateUnsuccessfulPayment(ctx app.Context, e app.Event) {
	reason := ([]string{
		"Недостаточно средств",
		"Предоставлен неверный пинкод",
		"Время ввода пинкода истекло",
	})[rand.Intn(3)]

	// successful
	// unsuccessful
	// canceled

	bb, err := json.Marshal(&PaymentEventRequest{
		Status: "unsuccessful",
		Reason: reason,
	})
	if err != nil {
		app.Log(err)
		return
	}

	res, err := http.Post(fmt.Sprintf("http://%s:8888/payment-gateway-api/order/%s/payment-event", host, c.Order.ID.String()),
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

func statusText(status string) app.UI {
	var text app.UI
	switch status {
	case "waiting_for_payment":
		text = app.P().Class("card-text", "text-primary").Style("font-size", "0.9em").Text("Ожидает оплаты")
	case "payment_timeout":
		text = app.P().Class("card-text", "text-danger").Style("font-size", "0.9em").Text("Таймаут оплаты")
	case "paid":
		text = app.P().Class("card-text", "text-primary").Style("font-size", "0.9em").Text("Оплачен")
	case "payment_canceled":
		text = app.P().Class("card-text", "text-danger").Style("font-size", "0.9em").Text("Отменён")
	case "cooking":
		text = app.P().Class("card-text", "text-primary").Style("font-size", "0.9em").Text("Готовится")
	case "ready":
		text = app.P().Class("card-text", "text-primary").Style("font-size", "0.9em").Text("Можно забирать")
	case "received":
		text = app.P().Class("card-text", "text-success").Style("font-size", "0.9em").Text("Отдан")
	}

	return text
}
