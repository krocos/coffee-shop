package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v9/pkg/app"
	"github.com/r3labs/sse/v2"
)

const host = "localhost"

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
		ID        uuid.UUID `json:"id"`
		Addr      string    `json:"addr"`
		KitchenID uuid.UUID `json:"kitchen_id"`
		CacheID   uuid.UUID `json:"cache_id"`
	}
	MenuResponse struct {
		Users  []*User  `json:"users"`
		Items  []*Item  `json:"items"`
		Points []*Point `json:"points"`
	}

	CacheOrderResponse struct {
		ID               uuid.UUID `json:"id"`
		OrderID          uuid.UUID `json:"order_id"`
		UserName         string    `json:"user_name"`
		Status           string    `json:"status"`
		ReadinessPercent int       `json:"readiness_percent"`
		CheckList        string    `json:"check_list"`
	}
)

type CacheUI struct {
	app.Compo
	Menu          *MenuResponse
	SelectedPoint *Point
	CacheOrders   []*CacheOrderResponse
}

func (u *CacheUI) OnMount(ctx app.Context) {
	ctx.Handle("cacheSelected", u.cacheSelectedAction)
	ctx.Handle("updateCacheOrders", u.updateCacheOrdersAction)

	ctx.Async(func() {
		res, err := http.Get(fmt.Sprintf("http://%s:8888/user-api/menu", host))
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

func (u *CacheUI) Render() app.UI {
	menuLoaded := u.Menu != nil && len(u.Menu.Users) > 0
	pointSelected := u.SelectedPoint != nil

	switch {
	case menuLoaded && !pointSelected:
		return app.Div().Class("container").Body(
			app.Div().Class("row").Body(
				app.Div().Class("col").Body(
					app.Br(),
					&CacheSelector{Points: u.Menu.Points},
				),
			),
		)
	case menuLoaded && pointSelected:
		return app.Div().Class("container").Body(
			app.Div().Class("row").Body(
				app.Div().Class("col").Body(
					app.Br(),
					app.H1().Text(fmt.Sprintf("?????????? ???? ?????????? %s", u.SelectedPoint.Addr)),
					app.P().Class("text-muted").Text(u.SelectedPoint.ID.String()),
				),
			),
			app.Div().Class("row").Body(
				app.Div().Class("col").Body(
					app.Range(u.CacheOrders).Slice(func(i int) app.UI {
						return &CacheOrderCompo{CacheOrder: u.CacheOrders[i]}
					}),
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

func (u *CacheUI) cacheSelectedAction(ctx app.Context, action app.Action) {
	u.SelectedPoint = action.Value.(*Point)

	ctx.NewAction("updateCacheOrders")

	ctx.Async(func() {
		client := sse.NewClient(fmt.Sprintf("http://%s:7995/cache/%s", host, u.SelectedPoint.CacheID.String()))
		err := client.SubscribeRaw(func(msg *sse.Event) {
			ctx.NewAction("updateCacheOrders")
		})
		if err != nil {
			app.Log(err)
			return
		}
	})
}

func (u *CacheUI) updateCacheOrdersAction(ctx app.Context, action app.Action) {
	orders, err := loadCacheOrders(u.SelectedPoint.CacheID)
	if err != nil {
		app.Log(err)
		return
	}

	u.CacheOrders = orders
}

func main() {
	app.Route("/", new(CacheUI))
	app.RunWhenOnBrowser()

	http.Handle("/", &app.Handler{
		Icon: app.Icon{
			Default:    "/web/favicon.ico",
			Large:      "/web/apple-touch-icon.png",
			AppleTouch: "/web/apple-touch-icon.png",
		},
		BackgroundColor: "#ffffff",
		ThemeColor:      "#ffffff",
		Title:           "Cache UI",
		Description:     "Cache UI to work with cache orders",
		RawHeaders: []string{
			`<link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet">
<script src="https://cdn.jsdelivr.net/npm/@popperjs/core@2.11.6/dist/umd/popper.min.js" defer></script>
<script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.min.js" defer></script>`,
		},
	})

	if err := http.ListenAndServe(":8092", nil); err != nil {
		panic(err)
	}
}

func loadCacheOrders(cacheID uuid.UUID) ([]*CacheOrderResponse, error) {
	res, err := http.Get(fmt.Sprintf("http://%s:8888/cache-api/cache/%s/orders", host, cacheID.String()))
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

	list := make([]*CacheOrderResponse, 0)
	if err = json.NewDecoder(res.Body).Decode(&list); err != nil {
		return nil, err
	}

	app.Log(list)

	return list, nil
}
