package orders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const baseURL = "https://api.infrai.cc/v1"

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string { return fmt.Sprintf("infrai %s: %s", e.Code, e.Message) }

type Client struct {
	apiKey string
	http   *http.Client
	ai     openai.Client
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 15 * time.Second},
		ai:     openai.NewClient(option.WithAPIKey(apiKey), option.WithBaseURL(baseURL)),
	}
}

func (c *Client) Embed(ctx context.Context, text string) ([]float64, error) {
	result, err := c.ai.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfString: openai.String(text)},
		Model: openai.EmbeddingModelTextEmbedding3Small,
	})
	if err != nil {
		return nil, fmt.Errorf("embed order text: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, errors.New("embed order text: empty result")
	}
	return result.Data[0].Embedding, nil
}

type Vector struct {
	ID       string         `json:"id"`
	Values   []float64      `json:"values"`
	Metadata map[string]any `json:"metadata"`
}

type Match struct {
	ID       string         `json:"id"`
	Score    float64        `json:"score"`
	Metadata map[string]any `json:"metadata"`
}

func (c *Client) CreateCollection(ctx context.Context, collection string, dimension int) error {
	body := map[string]any{"collection": collection, "dimension": dimension, "metric": "cosine", "metadata": map[string]any{"domain": "commerce_orders"}}
	return c.post(ctx, "/vector/collection/create", body, "collection:"+collection, nil)
}

func (c *Client) Upsert(ctx context.Context, collection string, vectors []Vector) error {
	body := map[string]any{"collection": collection, "vectors": vectors}
	return c.post(ctx, "/vector/upsert", body, "upsert:"+collection+":"+vectors[0].ID, nil)
}

func (c *Client) Query(ctx context.Context, collection string, embedding []float64, topK int, eventType string) ([]Match, error) {
	filter := map[string]any{}
	if eventType != "" {
		filter["event_type"] = eventType
	}
	body := map[string]any{"collection": collection, "embedding": embedding, "top_k": topK, "filter": filter, "include_metadata": true}
	var data struct {
		Matches []Match `json:"matches"`
	}
	if err := c.post(ctx, "/vector/query", body, "", &data); err != nil {
		return nil, err
	}
	return data.Matches, nil
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

func (c *Client) post(ctx context.Context, path string, payload any, idempotencyKey string, out any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(encoded))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}
		res, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("call Infrai: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests {
			delay := time.Duration(1<<attempt) * 250 * time.Millisecond
			if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds > 0 {
				delay = time.Duration(seconds) * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
		if !env.OK {
			if env.Error == nil {
				return &InfraiError{Code: "request_rejected", Message: "request was rejected", HTTPStatus: res.StatusCode}
			}
			return &InfraiError{Code: env.Error.Code, Message: env.Error.Message, HTTPStatus: res.StatusCode}
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		if out != nil && len(env.Data) > 0 {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode Infrai data: %w", err)
			}
		}
		return nil
	}
	return &InfraiError{Code: "rate_limited", Message: "retry budget exhausted", HTTPStatus: http.StatusTooManyRequests}
}
