package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v9/pkg/app"
	"github.com/r3labs/sse/v2"

	"github.com/krocos/coffee-shop/proxy"
)

const host = "localhost:8091"

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

	KitchenCookItemResponse struct {
		ID       uuid.UUID `json:"id"`
		Title    string    `json:"title"`
		Quantity float64   `json:"quantity"`
		OrderID  uuid.UUID `json:"order_id"`
	}
)

type KitchenUI struct {
	app.Compo
	Menu          *MenuResponse
	SelectedPoint *Point
	CookItems     []*KitchenCookItemResponse
}

func (u *KitchenUI) OnMount(ctx app.Context) {
	ctx.Handle("kitchenSelected", u.kitchenSelectedAction)
	ctx.Handle("updateKitchenCookItems", u.updateKitchenCookItemsAction)

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

func (u *KitchenUI) Render() app.UI {
	menuLoaded := u.Menu != nil && len(u.Menu.Users) > 0
	pointSelected := u.SelectedPoint != nil

	switch {
	case menuLoaded && !pointSelected:
		return app.Div().Class("container").Body(
			app.Div().Class("row").Body(
				app.Div().Class("col").Body(
					app.Br(),
					&KitchenSelector{Points: u.Menu.Points},
				),
			),
		)
	case menuLoaded && pointSelected:
		return app.Div().Class("container").Body(
			app.Div().Class("row").Body(
				app.Div().Class("col").Body(
					app.Br(),
					app.H1().Text(fmt.Sprintf("Кухня на улице %s", u.SelectedPoint.Addr)),
					app.P().Class("text-muted").Text(u.SelectedPoint.ID.String()),
				),
			),
			app.Div().Class("row").Body(
				app.Div().Class("col").Body(
					app.Range(u.CookItems).Slice(func(i int) app.UI {
						return &CookItemCompo{CookItem: u.CookItems[i]}
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

func (u *KitchenUI) kitchenSelectedAction(ctx app.Context, action app.Action) {
	u.SelectedPoint = action.Value.(*Point)

	ctx.NewAction("updateKitchenCookItems")

	ctx.Async(func() {
		client := sse.NewClient(fmt.Sprintf("http://%s/kitchen/%s", host, u.SelectedPoint.KitchenID.String()))
		err := client.SubscribeRaw(func(msg *sse.Event) {
			ctx.NewAction("updateKitchenCookItems")
		})
		if err != nil {
			app.Log(err)
			return
		}
	})
}

func (u *KitchenUI) updateKitchenCookItemsAction(ctx app.Context, action app.Action) {
	items, err := loadKitchenCookItems(u.SelectedPoint.KitchenID)
	if err != nil {
		app.Log(err)
		return
	}

	u.CookItems = items
}

func main() {
	app.Route("/", new(KitchenUI))
	app.RunWhenOnBrowser()

	http.Handle("/", &app.Handler{
		Icon: app.Icon{
			Default:    "/web/favicon.ico",
			Large:      "/web/apple-touch-icon.png",
			AppleTouch: "/web/apple-touch-icon.png",
		},
		BackgroundColor: "#ffffff",
		ThemeColor:      "#ffffff",
		Title:           "Kitchen UI",
		Description:     "Kitchen UI to work with cook items",
		RawHeaders: []string{
			`<link href="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/css/bootstrap.min.css" rel="stylesheet">
<script src="https://cdn.jsdelivr.net/npm/@popperjs/core@2.11.6/dist/umd/popper.min.js" defer></script>
<script src="https://cdn.jsdelivr.net/npm/bootstrap@5.2.2/dist/js/bootstrap.min.js" defer></script>`,
		},
	})

	if err := proxy.CreateServer(":8091").ListenAndServe(); err != nil {
		panic(err)
	}
}

func loadKitchenCookItems(kitchenID uuid.UUID) ([]*KitchenCookItemResponse, error) {
	res, err := http.Get(fmt.Sprintf("http://%s/kitchen-api/kitchen/%s/cook-items", host, kitchenID.String()))
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

	list := make([]*KitchenCookItemResponse, 0)
	if err = json.NewDecoder(res.Body).Decode(&list); err != nil {
		return nil, err
	}

	return list, nil
}
