package sse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

type ClientType string

const (
	clientTypeUser    ClientType = "user"
	clientTypeCache   ClientType = "cache"
	clientTypeKitchen ClientType = "kitchen"
)

type EventType string

const (
	eventOrderListUpdated           EventType = "order_list_updated"
	eventUnsuccessfulPayAttempt     EventType = "unsuccessful_pay_attempt"
	eventItemListUpdated            EventType = "item_list_updated"
	eventAttemptToEnterWrongPINCode EventType = "attempt_to_enter_wrong_pin_code"
)

type SSE struct {
	client *http.Client
	addr   *url.URL
}

func NewSSE(addr string) (*SSE, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	return &SSE{
		client: new(http.Client),
		addr:   u,
	}, nil
}

func (s *SSE) SendNotification(ctx context.Context, event Event) error {
	bb, err := json.Marshal(event)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.addr.String(), bytes.NewReader(bb))
	if err != nil {
		return err
	}

	res, err := s.client.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		bb, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("bad response with status '%d %s'",
				res.StatusCode, http.StatusText(res.StatusCode))
		}
		defer func() { _ = res.Body.Close() }()

		return fmt.Errorf("bad response with status '%d %s': %s",
			res.StatusCode, http.StatusText(res.StatusCode), string(bb))
	}

	return nil
}

type Event struct {
	ClientType ClientType `json:"client_type"`
	ClientID   uuid.UUID  `json:"client_id"`
	EventType  EventType  `json:"event_type"`
}

func NewOrderListUpdatedEvent() Event {
	return Event{EventType: eventOrderListUpdated}
}

func NewUnsuccessfulPayAttemptEvent() Event {
	return Event{EventType: eventUnsuccessfulPayAttempt}
}

func NewItemListUpdatedEvent() Event {
	return Event{EventType: eventItemListUpdated}
}

func NewAttemptToEnterWrongPINCodeEvent() Event {
	return Event{EventType: eventAttemptToEnterWrongPINCode}
}

func (e Event) ForUser() Event {
	e.ClientType = clientTypeUser
	return e
}

func (e Event) ForKitchen() Event {
	e.ClientType = clientTypeKitchen
	return e
}

func (e Event) ForCache() Event {
	e.ClientType = clientTypeCache
	return e
}

func (e Event) WithID(clientID uuid.UUID) Event {
	e.ClientID = clientID
	return e
}
