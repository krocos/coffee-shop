package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v9/pkg/app"
	"github.com/r3labs/sse/v2"

	"github.com/krocos/coffee-shop/proxy"
)

const host = "localhost:8090"

func init() {
	rand.Seed(time.Now().UnixMicro())
}

type (
	User struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}
	Item struct {
		ID    uuid.UUID `json:"id"`
		Title string    `json:"title"`
		Price float64   `json:"price"`
	}
	Point struct {
		ID   uuid.UUID `json:"id"`
		Addr string    `json:"addr"`
	}
	MenuResponse struct {
		Users  []*User  `json:"users"`
		Items  []*Item  `json:"items"`
		Points []*Point `json:"points"`
	}

	UserOrderResponse struct {
		ID         uuid.UUID                `json:"id"`
		CreatedAt  time.Time                `json:"created_at"`
		Status     string                   `json:"status"`
		TotalPrice float64                  `json:"total_price"`
		PINCode    string                   `json:"pin_code"`
		Point      *PointResponse           `json:"point"`
		Items      []*UserOrderItemResponse `json:"items"`
		LogItems   []*LogItemResponse       `json:"log_items"`
	}
	UserOrderItemResponse struct {
		ID         uuid.UUID `json:"id"`
		Title      string    `json:"title"`
		Price      float64   `json:"price"`
		Quantity   float64   `json:"quantity"`
		TotalPrice float64   `json:"total_price"`
	}
	LogItemResponse struct {
		ID   uuid.UUID `json:"id"`
		Text string    `json:"text"`
	}
	PointResponse struct {
		ID   uuid.UUID `json:"id"`
		Addr string    `json:"addr"`
	}
)

type UserUI struct {
	app.Compo
	Menu         *MenuResponse
	SelectedUser *User
	Orders       []*UserOrderResponse
}

func (u *UserUI) OnMount(ctx app.Context) {
	ctx.Handle("userSelected", u.userSelectedAction)
	ctx.Handle("updateOrders", u.updateOrdersAction)

	ctx.Async(func() {
		res, err := http.Get(fmt.Sprintf("http://%s/user-api/menu", host))
		if err != nil {
			app.Log(err)
			return
		}
		defer func() { _ = res.Body.Close() }()

		if res.StatusCode != http.StatusOK {
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

		menu := new(MenuResponse)
		if err = json.NewDecoder(res.Body).Decode(menu); err != nil {
			app.Log(err)
			return
		}

		ctx.Dispatch(func(ctx app.Context) {
			u.Menu = menu
		})
	})
}

func (u *UserUI) Render() app.UI {
	menuLoaded := u.Menu != nil && len(u.Menu.Users) > 0
	userSelected := u.SelectedUser != nil

	switch {
	case menuLoaded && !userSelected:
		return app.Div().Class("container").Body(
			app.Div().Class("row").Body(
				app.Div().Class("col").Body(
					app.Br(),
					NewUserSelector(u.Menu.Users),
				),
			),
		)
	case menuLoaded && userSelected:
		return app.Div().Class("container").Body(
			app.Div().Class("row", "justify-content-center").Body(
				app.Div().Class("col-sm-12", "col-md-9", "col-xl-12").Body(
					app.Br(),
					app.H1().Text(u.SelectedUser.Name),
					app.P().Class("text-muted").Text(u.SelectedUser.ID.String()),
				),
			),
			app.Div().Class("row", "justify-content-center").Body(
				app.Div().Class("col-sm-12", "col-md-9", "col-xl-6").Body(
					app.Br(),
					app.H2().Text("Меню"),
					NewUserOrderMaker(u.SelectedUser.ID, u.Menu.Items, u.Menu.Points),
				),
				app.Div().Class("col-sm-12", "col-md-9", "col-xl-6").Body(
					app.Br(),
					app.H2().Text("Заказы"),
					&OrderListing{Orders: u.Orders},
				),
			),
		)
	default:
		return app.Div().Class("container").Body(
			app.Div().Class("row").Body(
				app.Div().Class("col").Body(
					app.Br(),
					app.Div().Class("spinner-border", "text-primary"),
				),
			),
		)
	}
}

func (u *UserUI) userSelectedAction(ctx app.Context, action app.Action) {
	u.SelectedUser = action.Value.(*User)

	ctx.NewAction("updateOrders")

	ctx.Async(func() {
		client := sse.NewClient(fmt.Sprintf("http://%s/user/%s", host, u.SelectedUser.ID.String()))
		err := client.SubscribeRaw(func(msg *sse.Event) {
			ctx.NewAction("updateOrders")
		})
		if err != nil {
			app.Log(err)
			return
		}
	})
}

func (u *UserUI) updateOrdersAction(ctx app.Context, action app.Action) {
	orders, err := getUserOrders(u.SelectedUser.ID)
	if err != nil {
		app.Log(err)
		return
	}

	u.Orders = orders
}

func main() {
	app.Route("/", new(UserUI))
	app.RunWhenOnBrowser()

	http.Handle("/", &app.Handler{
		Name:      "Coffee-Shop",
		ShortName: "Coffee-Shop",
		Icon: app.Icon{
			Default:    "/web/favicon.ico",
			Large:      "/web/apple-touch-icon.png",
			AppleTouch: "/web/apple-touch-icon.png",
		},
		BackgroundColor: "#ffffff",
		ThemeColor:      "#ffffff",
		Title:           "Coffee-Shop",
		Description:     "Best coffee in the World",
		RawHeaders: []string{
			`<link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet">
<script src="https://cdn.jsdelivr.net/npm/@popperjs/core@2.11.6/dist/umd/popper.min.js" defer></script>
<script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.min.js" defer></script>`,
		},
	})

	if err := proxy.CreateServer(":8090").ListenAndServe(); err != nil {
		panic(err)
	}
}

func getUserOrders(userID uuid.UUID) ([]*UserOrderResponse, error) {
	res, err := http.Get(fmt.Sprintf("http://%s/user-api/user/%s/orders", host, userID.String()))
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		bb, err := io.ReadAll(res.Body)
		if err != nil {
			err = fmt.Errorf("bad response '%d %s'", res.StatusCode, http.StatusText(res.StatusCode))
			return nil, err
		}

		err = fmt.Errorf("bad response '%d %s': %s", res.StatusCode, http.StatusText(res.StatusCode), string(bb))
		return nil, err
	}

	list := make([]*UserOrderResponse, 0)
	if err = json.NewDecoder(res.Body).Decode(&list); err != nil {
		return nil, err
	}

	return list, nil
}
