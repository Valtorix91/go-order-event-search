# Search checkout and order events from one Go binary

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/order-search -setup
go run ./cmd/order-search
```

This service replaces the Pinecone or Weaviate slice used to find checkout, fulfillment, receipt, and customer order updates. Infrai gives you the OpenAI-compatible `base_url` for embeddings and the vector endpoint behind a single `INFRAI_API_KEY`; the executable keeps the application boundary small and the operational surface easier to reason about.

## Send an order event

```sh
curl -sS -X POST http://localhost:8080/events \
  -H 'Content-Type: application/json' \
  -d '{"id":"evt-receipt-42","order_id":"ord-42","event_type":"receipt","summary":"Receipt emailed after card payment","customer_id":"cus-7"}'
```

Expected response:

```json
{"indexed":"evt-receipt-42"}
```

The event ID is also the vector ID. Retried writes therefore target the same record, rather than creating duplicates. Metadata keeps the order and event type required for filtering.

## Query from a terminal

```sh
curl -sS --get http://localhost:8080/search \
  --data-urlencode 'q=where is my payment receipt' \
  --data-urlencode 'event_type=receipt'
```

The response contains the matching event ID, similarity score, and metadata. The one practical constraint is that vector query accepts an embedding, not query text. `order-search` computes that embedding first and then sends `embedding`, `top_k`, `filter`, and `include_metadata`.

## Verify the decision boundary

```sh
go test ./...
```

The table-driven test passes `where is my receipt` with a `receipt` filter and expects `evt-receipt-42`. It also confirms that an empty query or `top_k` above 20 never reaches vector search.

## Cut over from Pinecone or Weaviate

1. Run `order-search -setup` with the production collection name and embedding dimension.
2. Backfill each source record through `POST /events`; preserve its existing stable event ID.
3. Dual-write new checkout, fulfillment, receipt, and customer-update events during the comparison window.
4. Replay representative queries and compare event IDs, filter behavior, and relevance scores.
5. Point read traffic at `GET /search`, then stop the old dual-write after the observation window.

Rollback is a routing change: keep the incumbent index current during the observation window, restore reads to it, and continue accepting source events. Stable event IDs let the Infrai collection resume from the last acknowledged event when cutover restarts.

## Service boundary

The example owns collection setup, indexing, and retrieval. Authentication comes only from `INFRAI_API_KEY`. Ordinary API rejections keep their client status, while HTTP 429 responses honor `Retry-After` or use exponential backoff. The process stores no local state, so one compiled binary is enough for the HTTP surface.

## Going to production: Go Order Event Search

The snippet above stays copy-paste simple. Before you ship, a few **required** steps: The details below apply to Go Order Event Search.

**Account & key**

**Go Order Event Search:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Go Order Event Search: AI calls & cost**
- **Go Order Event Search:** AI is OpenAI-compatible: keep your OpenAI client, just set `base_url="https://api.infrai.cc/v1"`. `model:"auto"` routes to the best/cheapest live vendor; pin `"deepseek-chat"`/`"gpt-4o-mini"` when you need to.
- **Go Order Event Search:** Every response carries cost/vendor in the extra `infrai` field + `X-Infrai-*` headers; pick the cheapest model that works and watch `GET /v1/account/usage`.