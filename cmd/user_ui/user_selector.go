package main

import (
	"github.com/maxence-charriere/go-app/v9/pkg/app"
)

type UserSelector struct {
	app.Compo
	users []*User
}

func NewUserSelector(users []*User) *UserSelector {
	return &UserSelector{users: users}
}

func (s *UserSelector) Render() app.UI {
	return app.Div().Class("dropdown").Body(
		app.Button().Class("btn btn-primary dropdown-toggle").
			Type("button").Attr("data-bs-toggle", "dropdown").
			Text("Выбери пользователя"),
		app.Ul().Class("dropdown-menu").Body(
			app.Range(s.users).Slice(func(i int) app.UI {
				return app.Li().Body(
					app.A().Href("#").Class("dropdown-item").
						Text(s.users[i].Name).OnClick(s.selectUser(s.users[i])),
				)
			}),
		),
	)
}

func (s *UserSelector) selectUser(user *User) app.EventHandler {
	return func(ctx app.Context, e app.Event) {
		ctx.NewActionWithValue("userSelected", user)
	}
}
