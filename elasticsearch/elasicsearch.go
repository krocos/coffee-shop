package elasticsearch

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/olivere/elastic/v7"
)

type (
	Order struct {
		ID         string       `json:"id,omitempty"`
		CreatedAt  string       `json:"created_at,omitempty"`
		Status     string       `json:"status,omitempty"`
		TotalPrice float64      `json:"total_price,omitempty"`
		PINCode    string       `json:"pin_code,omitempty"`
		User       *User        `json:"user,omitempty"`
		Point      *Point       `json:"point,omitempty"`
		Items      []*OrderItem `json:"items,omitempty"`
		LogItems   []*LogItem   `json:"log_items,omitempty"`
	}
	User struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name,omitempty"`
	}
	Point struct {
		ID        string `json:"id,omitempty"`
		Addr      string `json:"addr,omitempty"`
		KitchenID string `json:"kitchen_id,omitempty"`
		CacheID   string `json:"cache_id,omitempty"`
	}
	OrderItem struct {
		ID         string  `json:"id,omitempty"`
		Title      string  `json:"title,omitempty"`
		Price      float64 `json:"price,omitempty"`
		ItemID     string  `json:"item_id,omitempty"`
		Quantity   float64 `json:"quantity,omitempty"`
		TotalPrice float64 `json:"total_price,omitempty"`
		OrderID    string  `json:"order_id,omitempty"`
	}
	LogItem struct {
		ID      string `json:"id,omitempty"`
		Text    string `json:"text,omitempty"`
		OrderID string `json:"order_id,omitempty"`
	}
)

type Config struct {
	Index string
	URL   string
}

type Search struct {
	indexName string
	mapping   string
	client    *elastic.Client
}

//go:embed mapping.json
var mapping string

func New(config Config) (*Search, error) {
	var err error
	s := &Search{
		indexName: config.Index,
		mapping:   mapping,
	}

	s.client, err = elastic.NewClient(
		elastic.SetURL(config.URL),
		elastic.SetSniff(false),
	)
	if err != nil {
		return nil, fmt.Errorf("make new elasticsearch client: %v", err)
	}

	return s, nil
}

func (s *Search) IsExists(ctx context.Context) (bool, error) {
	exists, err := s.client.IndexExists(s.indexName).Do(ctx)
	if err != nil {
		return false, fmt.Errorf("check that index '%s' exists %v", s.indexName, err)
	}

	return exists, nil
}

func (s *Search) CreateIndex(ctx context.Context) error {
	if _, err := s.client.CreateIndex(s.indexName).BodyString(s.mapping).Do(ctx); err != nil {
		return fmt.Errorf("create index %s: %v", s.indexName, err)
	}

	return nil
}

func (s *Search) DeleteIndex(ctx context.Context) error {
	if _, err := s.client.DeleteIndex(s.indexName).Do(ctx); err != nil {
		return fmt.Errorf("delete index %s: %v", s.indexName, err)
	}

	return nil
}

func (s *Search) IndexOrder(ctx context.Context, id uuid.UUID, doc *Order, refresh bool) error {
	indexService := s.client.Index().Id(id.String()).Index(s.indexName).BodyJson(doc)

	if refresh {
		indexService.Refresh("true")
	}

	if _, err := indexService.Do(ctx); err != nil {
		if refresh {
			return fmt.Errorf("index %s with refresh: %v", s.indexName, err)
		} else {
			return fmt.Errorf("index %s: %v", s.indexName, err)
		}
	}

	return nil
}

func (s *Search) UpdateOrder(ctx context.Context, id uuid.UUID, partialDoc *Order, refresh bool) error {
	updateService := s.client.Update().
		Index(s.indexName).
		Id(id.String()).
		Doc(partialDoc).
		DocAsUpsert(false)

	if refresh {
		updateService.Refresh("true")
	}

	if _, err := updateService.Do(ctx); err != nil {
		return fmt.Errorf("perform update: %v", err)
	}

	return nil
}

func (s *Search) ListUserOrders(ctx context.Context, userID uuid.UUID) ([]json.RawMessage, error) {
	res, err := s.client.Search(s.indexName).
		Query(elastic.NewTermQuery("user.id", userID.String())).Size(100).
		Sort("created_at", false).Do(ctx)
	if err != nil {
		return nil, err
	}

	list := make([]json.RawMessage, 0)

	for _, hit := range res.Hits.Hits {
		list = append(list, hit.Source)
	}

	return list, nil
}
