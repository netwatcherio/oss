# ClickHouse Vector Search for NetWatcher

This document explores how vector similarity search in ClickHouse could enhance NetWatcher's network monitoring and anomaly detection capabilities.

---

## Overview

ClickHouse 25.8+ provides production-ready **vector similarity search** using HNSW (Hierarchical Navigable Small World) indexes, enabling fast approximate nearest-neighbor queries on high-dimensional embeddings. This capability opens opportunities for AI-enhanced network monitoring without adding a separate vector database.

### Current State

| Aspect | Current | Required for Vector Search |
|--------|---------|---------------------------|
| ClickHouse Version | 24.x | **25.8+** |
| Probe Data Table | `probe_data` (MergeTree) | No change needed |
| Data Types | JSON payloads (`payload_raw`) | Add `Array(Float32)` embedding columns |

---

## Upgrade Path

### Version Requirements

```yaml
# docker-compose.yml - update image
clickhouse:
  image: clickhouse/clickhouse-server:25.8  # Minimum for vector_similarity index
```

Vector similarity search became **GA (General Availability)** in ClickHouse 25.8, bringing:
- HNSW indexes with configurable quantization (BFloat16, Int8)
- Binary quantization for reduced memory usage
- `QBit` data type for runtime precision tuning (25.10+)

---

## Use Cases for NetWatcher

### 1. MTR Route Similarity Search

**Problem**: Detecting unusual network paths or finding similar routing patterns across agents.

**Solution**: Embed MTR hop sequences as vectors to enable route similarity queries.

```sql
-- Schema extension for route embeddings
ALTER TABLE probe_data
ADD COLUMN route_embedding Array(Float32) CODEC(NONE),
ADD INDEX idx_route_embedding route_embedding 
    TYPE vector_similarity('hnsw', 'cosineDistance', 128) GRANULARITY 100000000;

-- Find similar routes to a reference trace
WITH [-0.12, 0.34, ...] AS ref_route  -- embedding of current route
SELECT 
    probe_id,
    created_at,
    target,
    cosineDistance(route_embedding, ref_route) AS distance
FROM probe_data
WHERE type = 'MTR' 
  AND created_at > now() - INTERVAL 7 DAY
ORDER BY distance
LIMIT 10;
```

**Embedding Strategy**:
- Convert hop IPs to numerical features (ASN, geolocation, hop position)
- Use a pre-trained network embedding model or train on historical routes
- Dimension: 64-256 floats depending on route complexity

---

### 2. Network Behavior Anomaly Detection

**Problem**: Identifying network anomalies based on patterns across multiple metrics (latency, packet loss, jitter).

**Solution**: Create multi-metric embeddings for each probe result, then find outliers using distance from cluster centroids.

```sql
-- Create embeddings table for behavioral patterns
CREATE TABLE probe_embeddings (
    probe_id        UInt64,
    agent_id        UInt64,
    created_at      DateTime('UTC'),
    type            LowCardinality(String),
    embedding       Array(Float32) CODEC(NONE),
    INDEX idx_embed embedding TYPE vector_similarity('hnsw', 'L2Distance', 64)
) ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (type, agent_id, created_at)
TTL created_at + INTERVAL 30 DAY DELETE;

-- Find anomalous PING behavior compared to normal baseline
WITH [0.1, 0.05, 45.2, ...] AS normal_baseline  -- avg embedding for "healthy" pings
SELECT 
    agent_id,
    probe_id,
    L2Distance(embedding, normal_baseline) AS anomaly_score
FROM probe_embeddings
WHERE type = 'PING' 
  AND created_at > now() - INTERVAL 1 HOUR
  AND L2Distance(embedding, normal_baseline) > 2.5  -- threshold
ORDER BY anomaly_score DESC
LIMIT 50;
```

**Embedding Features for PING**:
- `avg_rtt` (normalized)
- `min_rtt` / `max_rtt` (normalized)
- `packet_loss` (percentage)
- `jitter` (std deviation of RTT)
- Time-based features (hour of day, day of week)

---

### 3. TrafficSim Pattern Matching

**Problem**: Finding similar network performance patterns for capacity planning and SLA monitoring.

**Solution**: Embed TrafficSim results (throughput, latency, packet metrics) as vectors.

```sql
-- Query: Find time periods with similar network performance
WITH [...] AS current_performance
SELECT 
    created_at,
    probe_id,
    L2Distance(perf_embedding, current_performance) AS similarity
FROM trafficsim_embeddings
WHERE agent_id = 42
ORDER BY similarity
LIMIT 10;
```

---

### 4. Agent Fingerprinting / Clustering

**Problem**: Grouping agents with similar network characteristics for baseline comparison.

**Solution**: Create agent-level embeddings based on aggregated probe results.

```sql
-- Cluster agents by network behavior
CREATE TABLE agent_fingerprints (
    agent_id         UInt64,
    updated_at       DateTime('UTC'),
    fingerprint      Array(Float32) CODEC(NONE),
    INDEX idx_fp fingerprint TYPE vector_similarity('hnsw', 'cosineDistance', 128)
) ENGINE = ReplacingMergeTree(updated_at)
ORDER BY agent_id;

-- Find agents similar to a problematic one
WITH (SELECT fingerprint FROM agent_fingerprints WHERE agent_id = 123) AS problem_fp
SELECT 
    agent_id,
    cosineDistance(fingerprint, problem_fp) AS similarity
FROM agent_fingerprints
WHERE agent_id != 123
ORDER BY similarity
LIMIT 10;
```

---

## Implementation Architecture

### Embedding Generation Pipeline

```
┌─────────────┐    ┌──────────────────┐    ┌─────────────────┐
│  Agent      │───▶│  Controller      │───▶│  Embedding      │
│  Probe Data │    │  (probe_data)    │    │  Service        │
└─────────────┘    └──────────────────┘    └────────┬────────┘
                                                     │
                           ┌─────────────────────────▼────────────────┐
                           │                                          │
                   ┌───────▼────────┐                    ┌────────────▼───────┐
                   │  Local Model   │                    │  External API      │
                   │  (sentence-    │                    │  (OpenAI, Vertex)  │
                   │  transformers) │                    │                    │
                   └───────┬────────┘                    └────────────┬───────┘
                           │                                          │
                           └─────────────────┬────────────────────────┘
                                             │
                                    ┌────────▼────────┐
                                    │  ClickHouse     │
                                    │  (embeddings    │
                                    │   tables)       │
                                    └─────────────────┘
```

### Option A: Go-Native Embedding (Recommended for Low Latency)

```go
// internal/embedding/embedding.go
package embedding

import (
    "github.com/nlpodyssey/spago/pkg/nlp/transformers/bert"
)

type Embedder interface {
    Embed(probeData *probe.ProbeData) ([]float32, error)
}

// PingEmbedder creates embeddings from PING probe results
type PingEmbedder struct {
    // Normalization parameters learned from historical data
    meanLatency, stdLatency float64
}

func (e *PingEmbedder) Embed(data *probe.ProbeData) ([]float32, error) {
    var payload pingPayload
    json.Unmarshal(data.Payload, &payload)
    
    // Create fixed-dimension embedding
    return []float32{
        normalize(payload.AvgRtt, e.meanLatency, e.stdLatency),
        normalize(payload.MinRtt, e.meanLatency, e.stdLatency),
        normalize(payload.MaxRtt, e.meanLatency, e.stdLatency),
        float32(payload.PacketLoss) / 100.0,
        // ... additional features
    }, nil
}
```

### Option B: External Embedding Service

```go
// Use OpenAI embeddings for text-heavy probes (DNS queries, error messages)
func embedWithOpenAI(text string) ([]float32, error) {
    resp, err := openai.CreateEmbedding(context.Background(), openai.EmbeddingRequest{
        Model: openai.SmallEmbedding3,
        Input: []string{text},
    })
    return resp.Data[0].Embedding, err
}
```

---

## Schema Migration Plan

### Phase 1: Upgrade ClickHouse (v25.8+)

```bash
# 1. Update docker-compose.yml
# 2. Backup existing data
docker exec clickhouse clickhouse-client --query "BACKUP TABLE probe_data TO Disk('backups', 'probe_data.zip')"

# 3. Upgrade container
docker-compose pull clickhouse
docker-compose up -d clickhouse
```

### Phase 2: Add Embedding Columns

```sql
-- Add embedding column to existing table
ALTER TABLE probe_data
ADD COLUMN IF NOT EXISTS embedding Array(Float32) DEFAULT [] CODEC(NONE);

-- Create vector similarity index (will be built on new inserts)
ALTER TABLE probe_data
ADD INDEX idx_embedding embedding 
    TYPE vector_similarity('hnsw', 'L2Distance', 64) GRANULARITY 100000000;

-- Materialize index for existing data (can be slow for large tables)
ALTER TABLE probe_data MATERIALIZE INDEX idx_embedding SETTINGS mutations_sync = 2;
```

### Phase 3: Deploy Embedding Service

```yaml
# docker-compose.yml additions
embedding-worker:
  build: ./embedding-worker
  environment:
    CLICKHOUSE_HOST: clickhouse
    EMBEDDING_MODEL: "all-MiniLM-L6-v2"  # For local model
  depends_on:
    - clickhouse
```

---

## Performance Considerations

### Index Configuration

| Parameter | Default | Recommendation |
|-----------|---------|----------------|
| `GRANULARITY` | 100M | Keep default for tables <1B rows |
| `hnsw_max_connections_per_layer` | 16 | Increase to 32 for higher recall |
| `hnsw_candidate_list_size_for_construction` | 128 | Increase to 256 for better quality |
| Quantization | `f32` | Use `f16` or `bf16` for 2x memory savings |

### Memory & Storage

```sql
-- Estimate storage for embeddings
-- 1M rows × 64 dimensions × 4 bytes = ~256 MB uncompressed
-- With CODEC(NONE), storage ≈ raw size (vectors don't compress well)

SELECT
    table,
    formatReadableSize(sum(data_compressed_bytes)) AS compressed,
    formatReadableSize(sum(data_uncompressed_bytes)) AS uncompressed
FROM system.columns
WHERE table = 'probe_data' AND name = 'embedding'
GROUP BY table;
```

### Query Performance Tips

1. **Pre-filter before vector search**: Use WHERE clauses to reduce candidate set
2. **Use appropriate LIMIT**: Larger limits reduce speedup from ANN indexes
3. **Disable rescoring for speed**: `SET vector_search_with_rescoring = 0`

```sql
-- Optimal query pattern
SELECT probe_id, L2Distance(embedding, [...]) AS dist
FROM probe_data
WHERE type = 'PING'                    -- Pre-filter by type
  AND created_at > now() - INTERVAL 1 DAY  -- Time window
ORDER BY dist
LIMIT 100                              -- Keep reasonable limit
SETTINGS vector_search_with_rescoring = 0;
```

---

## Alternative: Hybrid Approach with pgvector

If immediate vector search is needed without upgrading ClickHouse, consider using PostgreSQL with pgvector for embedding storage while keeping time-series data in ClickHouse:

```sql
-- PostgreSQL with pgvector
CREATE EXTENSION vector;

CREATE TABLE probe_embeddings (
    probe_id BIGINT REFERENCES probes(id),
    embedding vector(64),
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX ON probe_embeddings USING ivfflat (embedding vector_l2_ops);
```

This adds complexity but works with the current stack.

---

## Recommended Roadmap

| Phase | Effort | Description |
|-------|--------|-------------|
| **1. Upgrade ClickHouse** | Low | Update to 25.8+ in dev environment |
| **2. POC: PING Embeddings** | Medium | Add simple embedding pipeline for PING probes |
| **3. Anomaly Detection MVP** | Medium | Implement distance-based alerting |
| **4. MTR Route Similarity** | High | Full route embedding with ML model |
| **5. Agent Clustering** | Medium | Behavioral fingerprinting for agents |

---

## References

- [ClickHouse Vector Similarity Indexes](https://clickhouse.com/docs/en/engines/table-engines/mergetree-family/annindexes)
- [QBit Data Type (25.10+)](https://clickhouse.com/docs/sql-reference/data-types/qbit)
- [HNSW Algorithm Paper](https://arxiv.org/abs/1603.09320)
- [ClickHouse 2025 Roadmap](https://github.com/ClickHouse/ClickHouse/issues/roadmap-2025)
