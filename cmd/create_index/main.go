package main

import (
	"context"

	"github.com/krocos/coffee-shop/elasticsearch"
)

func main() {
	search, err := elasticsearch.New(elasticsearch.Config{
		Index: "coffee_shop_search_index",
		URL:   "http://localhost:9202",
	})
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	exists, err := search.IsExists(ctx)
	if err != nil {
		panic(err)
	}

	if exists {
		if err = search.DeleteIndex(ctx); err != nil {
			panic(err)
		}
	}

	if err = search.CreateIndex(ctx); err != nil {
		panic(err)
	}
}
