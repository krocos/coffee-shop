package main

import (
	"log"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/krocos/coffee-shop/backend"
	"github.com/krocos/coffee-shop/elasticsearch"
	"github.com/krocos/coffee-shop/postgres"
	"github.com/krocos/coffee-shop/sse"
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

	newSSE, err := sse.NewSSE("http://localhost:7995/send-event")
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

	w := worker.New(c, "coffee", worker.Options{})

	w.RegisterWorkflow(backend.OrderWorkflow)
	w.RegisterActivity(postgres.NewPostgres(db))
	w.RegisterActivity(newSSE)
	w.RegisterActivity(search)

	if err = w.Run(worker.InterruptCh()); err != nil {
		log.Println(err)
	}
}
