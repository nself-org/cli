# Search Command

> **⚠️ DEPRECATED**: `nself search` is deprecated and will be removed in v1.0.0.
> Please use `nself service search` instead.
> Run `nself service search --help` for full usage information.

Configure and manage search services for your nself project.

## Quick Start

```bash
# Enable search (uses PostgreSQL full-text by default)
nself search enable

# Or use a dedicated search engine
nself search enable --engine meilisearch

# Check status
nself search status

# Test search
nself search test "hello world"
```

## Commands

| Command | Description |
|---------|-------------|
| `nself search` | Show help |
| `nself search enable [--engine <name>]` | Enable search service |
| `nself search disable` | Disable search service |
| `nself search status` | Show search service status |
| `nself search test [query]` | Test search functionality |
| `nself search reindex` | Rebuild search index |
| `nself search configure` | Configure search settings |
| `nself search setup` | Interactive setup wizard |

## Supported Engines

| Engine | Best For | RAM Usage |
|--------|----------|-----------|
| **PostgreSQL** | Simple full-text, zero extra services | ~0 (uses existing DB) |
| **MeiliSearch** | Typo-tolerant, instant search | ~100-500MB |
| **Typesense** | Fast, typo-tolerant search | ~100-500MB |
| **Elasticsearch** | Complex queries, analytics | 1-4GB |
| **OpenSearch** | Elasticsearch alternative (AWS) | 1-4GB |
| **Sonic** | Lightweight, fast ingestion | ~50-100MB |

## PostgreSQL Full-Text Search

The simplest option - uses your existing PostgreSQL database:

```bash
nself search enable --engine postgres
```

**SQL Example:**
```sql
-- Create a full-text index
CREATE INDEX articles_fts ON articles
USING gin(to_tsvector('english', title || ' ' || content));

-- Search
SELECT * FROM articles
WHERE to_tsvector('english', title || ' ' || content)
   @@ plainto_tsquery('english', 'search term');
```

## MeiliSearch

Fast, typo-tolerant search with instant results:

```bash
nself search enable --engine meilisearch
```

**JavaScript Example:**
```javascript
const { MeiliSearch } = require('meilisearch');

const client = new MeiliSearch({
  host: 'http://search:7700',
  apiKey: process.env.SEARCH_API_KEY
});

// Index documents
await client.index('products').addDocuments([
  { id: 1, title: 'iPhone', description: 'Apple smartphone' },
  { id: 2, title: 'Galaxy', description: 'Samsung smartphone' }
]);

// Search
const results = await client.index('products').search('phone');
```

## Typesense

High-performance search with typo tolerance:

```bash
nself search enable --engine typesense
```

**JavaScript Example:**
```javascript
const Typesense = require('typesense');

const client = new Typesense.Client({
  nodes: [{ host: 'search', port: 8108, protocol: 'http' }],
  apiKey: process.env.SEARCH_API_KEY
});

// Create collection
await client.collections().create({
  name: 'products',
  fields: [
    { name: 'title', type: 'string' },
    { name: 'price', type: 'float' }
  ]
});

// Search
const results = await client.collections('products')
  .documents()
  .search({ q: 'phone', query_by: 'title' });
```

## Elasticsearch / OpenSearch

Full-featured search and analytics:

```bash
nself search enable --engine elasticsearch
# or
nself search enable --engine opensearch
```

**JavaScript Example:**
```javascript
const { Client } = require('@elastic/elasticsearch');

const client = new Client({ node: 'http://search:9200' });

// Index document
await client.index({
  index: 'products',
  document: { title: 'iPhone', price: 999 }
});

// Search
const results = await client.search({
  index: 'products',
  body: {
    query: { match: { title: 'phone' } }
  }
});
```

## Sonic

Minimal memory footprint, great for simple search:

```bash
nself search enable --engine sonic
```

**Note:** Sonic uses a custom TCP protocol, not HTTP. Use the official client libraries.

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SEARCH_ENABLED` | Enable search service | `false` |
| `SEARCH_ENGINE` | Search engine to use | `postgres` |
| `SEARCH_HOST` | Search service hostname | `search` |
| `SEARCH_PORT` | Search service port | varies by engine |
| `SEARCH_API_KEY` | API key (if required) | - |
| `SEARCH_INDEX_PREFIX` | Prefix for index names | `${PROJECT_NAME}_` |

## Testing Your Setup

```bash
# Run connectivity and functionality test
nself search test

# Test with a specific query
nself search test "your search query"
```

## Reindexing

When you need to rebuild your search index:

```bash
nself search reindex
```

This provides engine-specific instructions for reindexing your data.

## Comparison Guide

| Feature | PostgreSQL | MeiliSearch | Typesense | Elasticsearch |
|---------|------------|-------------|-----------|---------------|
| Typo Tolerance | No | Yes | Yes | Plugin |
| Faceted Search | Manual | Yes | Yes | Yes |
| Geosearch | PostGIS | Yes | Yes | Yes |
| Analytics | No | No | No | Yes |
| Setup Complexity | None | Low | Low | High |
| Memory Usage | Shared | Low | Low | High |

## Recommendations

- **Simple sites**: PostgreSQL full-text search
- **E-commerce**: MeiliSearch or Typesense
- **Complex analytics**: Elasticsearch
- **Resource constrained**: Sonic
- **AWS environment**: OpenSearch
