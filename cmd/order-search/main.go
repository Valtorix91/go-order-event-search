package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/example/order-event-search/internal/orders"
)

func main() {
	listen := flag.String("listen", ":8080", "HTTP listen address")
	collection := flag.String("collection", "order-events", "vector collection")
	dimension := flag.Int("dimension", 1536, "embedding dimension")
	setup := flag.Bool("setup", false, "create the vector collection and exit")
	flag.Parse()

	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	client := orders.NewClient(key)
	if *setup {
		if err := client.CreateCollection(context.Background(), *collection, *dimension); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("collection %q is ready\n", *collection)
		return
	}

	search := orders.NewSearch(client, *collection)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /events", func(w http.ResponseWriter, r *http.Request) {
		var event orders.Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil || event.ID == "" || event.Summary == "" {
			http.Error(w, "id and summary are required", http.StatusBadRequest)
			return
		}
		if err := search.Index(r.Context(), event); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"indexed": event.ID})
	})
	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		matches, err := search.Find(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("event_type"), 5)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"matches": matches})
	})
	log.Printf("order search listening on %s", *listen)
	log.Fatal(http.ListenAndServe(*listen, mux))
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	var infraiErr *orders.InfraiError
	if errors.As(err, &infraiErr) && infraiErr.HTTPStatus >= 400 && infraiErr.HTTPStatus < 500 {
		status = infraiErr.HTTPStatus
	} else if err.Error() == "query is required" || err.Error() == "top_k must be between 1 and 20" {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
