package main

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"

	"github.com/krocos/coffee-shop/postgres"
)

func main() {
	db, err := postgres.NewGorm(postgres.GormConfig{
		Host:     "localhost",
		Port:     "5442",
		Database: "postgres",
		Username: "postgres",
		Password: "postgres",
	})
	if err != nil {
		panic(err)
	}

	err = db.AutoMigrate(
		postgres.User{},
		postgres.Item{},
		postgres.Point{},
		postgres.Order{},
		postgres.OrderItem{},
		postgres.LogItem{},
		postgres.CookItem{},
		postgres.CacheOrder{},
	)
	if err != nil {
		panic(err)
	}

	users := []*postgres.User{
		{Name: "Иван Иванович"},
		{Name: "Ватева Ватевович"},
		{Name: "Василий Петрович"},
	}

	items := []*postgres.Item{
		{Title: "Латте", Price: 95.50},
		{Title: "Латте c сиропом", Price: 105.50},
		{Title: "Еспрессо", Price: 89.95},
		{Title: "Двойной еспрессо", Price: 115.45},
		{Title: "Ристретто", Price: 74.95},
		{Title: "Пончики", Price: 49.95},
	}

	points := []*postgres.Point{
		{Addr: "Татищева 49"},
		{Addr: "Академика Бардина 32/1"},
		{Addr: "Банковский переулок 10"},
	}

	if err = db.Create(users).Error; err != nil {
		panic(err)
	}

	if err = db.Create(items).Error; err != nil {
		panic(err)
	}

	if err = db.Create(points).Error; err != nil {
		panic(err)
	}

	fmt.Println("USERS")
	fmt.Println()

	spew.Dump(users)

	fmt.Println()
	fmt.Println("ITEMS")
	fmt.Println()

	spew.Dump(items)

	fmt.Println()
	fmt.Println("POINTS")
	fmt.Println()

	spew.Dump(points)
}
