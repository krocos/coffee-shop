package main

import (
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/krocos/coffee-shop/elasticsearch"
	"github.com/krocos/coffee-shop/handling"
	"github.com/krocos/coffee-shop/postgres"
	"github.com/krocos/coffee-shop/zapadapter"
)

func main() {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.TimeKey = "time"
	config.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	config.Level.SetLevel(zap.DebugLevel)

	logger, err := config.Build()
	if err != nil {
		panic(err)
	}

	search, err := elasticsearch.New(elasticsearch.Config{
		Index: "coffee_shop_search_index",
		URL:   "http://localhost:9202",
	})
	if err != nil {
		panic(err)
	}

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

	c, err := client.Dial(client.Options{
		HostPort:  client.DefaultHostPort,
		Namespace: client.DefaultNamespace,
		Logger:    zapadapter.NewZapAdapter(logger),
	})
	if err != nil {
		panic(err)
	}
	defer c.Close()

	h := handling.NewHandling(c, postgres.NewPostgres(db), search)
	router := mux.NewRouter()

	router.HandleFunc("/user-api/menu", h.GetMenu).Methods(http.MethodGet)
	router.HandleFunc("/user-api/order", h.CreateOrder).Methods(http.MethodPost)
	router.HandleFunc("/user-api/user/{user_id}/orders", h.ListUserOrders).Methods(http.MethodGet)

	router.HandleFunc("/payment-gateway-api/order/{order_id}/payment-event", h.PaymentEvent).Methods(http.MethodPost)
	router.HandleFunc("/kitchen-api/order/{order_id}/item-cooked", h.OrderItemCooked).Methods(http.MethodPost)
	router.HandleFunc("/kitchen-api/kitchen/{kitchen_id}/cook-items", h.ListKitchenCookItems).Methods(http.MethodGet)
	router.HandleFunc("/cache-api/order/{order_id}/receive-order", h.ReceiveOrder).Methods(http.MethodPost)
	router.HandleFunc("/cache-api/cache/{cache_id}/orders", h.ListCacheOrders).Methods(http.MethodGet)

	if err = http.ListenAndServe(":8888", cors.AllowAll().Handler(router)); err != nil {
		log.Println(err)
	}
}
