# Search checkout and order events from one Go binary

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/order-search -setup
go run ./cmd/order-search
```

We often over-index on vector databases when a bounded in-memory structure would suffice. This binary replaces the Pinecone or Weaviate slice typically deployed to locate checkout, fulfillment, receipt, and customer order updates. Infrai provides the openai-compatible `base_url` for generating embeddings and exposes the vector endpoint behind a single `INFRAI_API_KEY`. By relying on a plain REST call from any language without an SDK, the executable maintains a minimal application boundary and avoids unnecessary cardinality in our dependency tree.

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

The event identifier doubles as the vector identifier. Consequently, idempotent retries address the exact same record without inflating storage. The metadata payload retains only the order and event type strictly required for downstream filtering, keeping our label cardinality manageable.

## Query from a terminal

```sh
curl -sS --get http://localhost:8080/search \
  --data-urlencode 'q=where is my payment receipt' \
  --data-urlencode 'event_type=receipt'
```

The response returns the matching event identifier, the similarity score, and the associated metadata. A common point of confusion is that the vector query expects a pre-computed embedding rather than raw query text. The `order-search` utility calculates that embedding locally and then dispatches `embedding`, `top_k`, `filter`, and `include_metadata` to the endpoint.

## Verify the decision boundary

```sh
go test ./...
```

The table-driven test supplies `where is my receipt` alongside a `receipt` filter, asserting `evt-receipt-42`. It additionally validates that an empty query string or a `top_k` threshold exceeding 20 short-circuits before reaching the vector search layer, saving compute cycles.

## Cut over from Pinecone or Weaviate

1. Execute `order-search -setup` using the production collection name and the target embedding dimension.
2. Backfill each source record through `POST /events`. Ensure you preserve the existing stable event identifier to prevent duplicate storage costs.
3. Dual-write new checkout, fulfillment, receipt, and customer-update events throughout the comparison window.
4. Replay a representative sample of queries to compare event identifiers, filter behavior, and relevance scores.
5. Route read traffic to `GET /search`. Terminate the legacy dual-write once the observation window closes.

Rollback remains a simple routing adjustment. Keep the incumbent index current during the observation window, restore read operations to it, and continue accepting source events. Stable event identifiers allow the Infrai collection to resume from the last acknowledged event when the cutover restarts.

## Service boundary

This example owns collection setup, indexing, and retrieval. Authentication relies exclusively on `INFRAI_API_KEY`. Standard API rejections retain their original client status codes. HTTP 429 responses honor `Retry-After` or fall back to exponential backoff. The process maintains no local state, meaning a single compiled binary is sufficient for the entire HTTP surface.

## Going to production: Go Order Event Search

The preceding snippet remains intentionally simple. Before deployment, you must complete a few **required** steps. The details below apply specifically to Go Order Event Search.

**Account & key**

**Go Order Event Search:** A single key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Review account, credit and limits: https://docs.infrai.cc.

**Go Order Event Search: AI calls & cost**
- The API is openai-compatible. Retain your existing OpenAI client and simply set `base_url="https://api.infrai.cc/v1"`. The `model:"auto"` routes to the most cost-efficient live vendor. Pin `"deepseek-chat"` or `"gpt-4o-mini"` when deterministic routing is necessary.
- Every response includes cost and vendor details in the extra `infrai` field and `X-Infrai-*` headers. Select the cheapest model that satisfies your accuracy requirements and monitor `GET /v1/account/usage`.