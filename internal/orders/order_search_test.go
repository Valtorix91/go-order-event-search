package orders

import (
	"context"
	"reflect"
	"testing"
)

type fakeBackend struct {
	queryCalls int
	filter     string
}

func (f *fakeBackend) Embed(context.Context, string) ([]float64, error) {
	return []float64{0.2, 0.8}, nil
}
func (f *fakeBackend) Upsert(context.Context, string, []Vector) error { return nil }
func (f *fakeBackend) Query(_ context.Context, _ string, _ []float64, _ int, filter string) ([]Match, error) {
	f.queryCalls++
	f.filter = filter
	return []Match{{ID: "evt-receipt-42", Score: 0.94}}, nil
}

func TestFindOrderEvents(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		topK      int
		filter    string
		wantIDs   []string
		wantError bool
	}{
		{name: "receipt intent reaches vector search", query: "where is my receipt", topK: 3, filter: "receipt", wantIDs: []string{"evt-receipt-42"}},
		{name: "empty query stops before backend", query: "", topK: 3, wantError: true},
		{name: "oversized result set stops before backend", query: "order status", topK: 21, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := &fakeBackend{}
			search := NewSearch(backend, "order-events")
			matches, err := search.Find(context.Background(), tt.query, tt.filter, tt.topK)
			if (err != nil) != tt.wantError {
				t.Fatalf("Find() error = %v, wantError %v", err, tt.wantError)
			}
			if tt.wantError {
				if backend.queryCalls != 0 {
					t.Fatal("invalid request reached vector backend")
				}
				return
			}
			var ids []string
			for _, match := range matches {
				ids = append(ids, match.ID)
			}
			if !reflect.DeepEqual(ids, tt.wantIDs) || backend.filter != tt.filter {
				t.Fatalf("ids/filter = %v/%q, want %v/%q", ids, backend.filter, tt.wantIDs, tt.filter)
			}
		})
	}
}
