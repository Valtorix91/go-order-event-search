package orders

import (
	"context"
	"fmt"
)

type Event struct {
	ID         string `json:"id"`
	OrderID    string `json:"order_id"`
	EventType  string `json:"event_type"`
	Summary    string `json:"summary"`
	CustomerID string `json:"customer_id"`
}

type Backend interface {
	Embed(context.Context, string) ([]float64, error)
	Upsert(context.Context, string, []Vector) error
	Query(context.Context, string, []float64, int, string) ([]Match, error)
}

type Search struct {
	backend    Backend
	collection string
}

func NewSearch(backend Backend, collection string) *Search {
	return &Search{backend: backend, collection: collection}
}

func (s *Search) Index(ctx context.Context, event Event) error {
	embedding, err := s.backend.Embed(ctx, event.Summary)
	if err != nil {
		return err
	}
	vector := Vector{ID: event.ID, Values: embedding, Metadata: map[string]any{
		"order_id": event.OrderID, "event_type": event.EventType, "summary": event.Summary, "customer_id": event.CustomerID,
	}}
	return s.backend.Upsert(ctx, s.collection, []Vector{vector})
}

func (s *Search) Find(ctx context.Context, query, eventType string, topK int) ([]Match, error) {
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if topK < 1 || topK > 20 {
		return nil, fmt.Errorf("top_k must be between 1 and 20")
	}
	embedding, err := s.backend.Embed(ctx, query)
	if err != nil {
		return nil, err
	}
	return s.backend.Query(ctx, s.collection, embedding, topK, eventType)
}
