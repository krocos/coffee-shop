package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	ssse "github.com/r3labs/sse/v2"
	"github.com/rs/cors"

	localSse "github.com/krocos/coffee-shop/sse"
)

func main() {

	server := ssse.New()
	server.AutoStream = true
	server.AutoReplay = false
	server.OnSubscribe = func(streamID string, sub *ssse.Subscriber) {
		log.Println(fmt.Sprintf("connected to '%s' via '%s'", streamID, sub.URL.String()))
	}
	server.OnUnsubscribe = func(streamID string, sub *ssse.Subscriber) {
		log.Println(fmt.Sprintf("disconnected to '%s' via '%s'", streamID, sub.URL.String()))
	}

	router := mux.NewRouter()

	router.HandleFunc("/user/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		clientID, err := uuid.Parse(mux.Vars(r)["user_id"])
		if err != nil {
			http.Error(w, fmt.Errorf("parse user id: %v", err).Error(), http.StatusBadRequest)
			return
		}

		vv := r.URL.Query()
		vv.Add("stream", fmt.Sprintf("user:%s", clientID.String()))
		r.URL.RawQuery = vv.Encode()

		server.ServeHTTP(w, r)
	})

	router.HandleFunc("/kitchen/{kitchen_id}", func(w http.ResponseWriter, r *http.Request) {
		clientID, err := uuid.Parse(mux.Vars(r)["kitchen_id"])
		if err != nil {
			http.Error(w, fmt.Errorf("parse user id: %v", err).Error(), http.StatusBadRequest)
			return
		}

		vv := r.URL.Query()
		vv.Add("stream", fmt.Sprintf("kitchen:%s", clientID.String()))
		r.URL.RawQuery = vv.Encode()

		server.ServeHTTP(w, r)
	})

	router.HandleFunc("/cache/{cache_id}", func(w http.ResponseWriter, r *http.Request) {
		clientID, err := uuid.Parse(mux.Vars(r)["cache_id"])
		if err != nil {
			http.Error(w, fmt.Errorf("parse user id: %v", err).Error(), http.StatusBadRequest)
			return
		}

		vv := r.URL.Query()
		vv.Add("stream", fmt.Sprintf("cache:%s", clientID.String()))
		r.URL.RawQuery = vv.Encode()

		server.ServeHTTP(w, r)
	})

	router.HandleFunc("/send-event", func(w http.ResponseWriter, r *http.Request) {
		event := new(localSse.Event)
		if err := json.NewDecoder(r.Body).Decode(event); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		go func() {
			streamID := fmt.Sprintf("%s:%s", event.ClientType, event.ClientID.String())

			switch event.ClientType {
			case "user", "cache", "kitchen":
				server.Publish(streamID, &ssse.Event{Data: []byte(event.EventType)})
			}
		}()
	})

	if err := http.ListenAndServe(":7995", cors.AllowAll().Handler(router)); err != nil {
		panic(err)
	}
}
