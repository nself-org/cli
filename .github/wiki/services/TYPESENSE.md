# Typesense Search Service

## Overview

Typesense is a fast, typo-tolerant search engine built for instant search experiences. It's an open-source alternative to Algolia and offers better performance than Elasticsearch for most use cases with significantly lower resource requirements.

## Key Features

- **Blazing Fast**: Sub-50ms search latency
- **Typo Tolerance**: Automatically handles typos in queries
- **Faceted Search**: Filter and refine results
- **Geo Search**: Location-based search with distance
- **Vector Search**: Semantic search with embeddings
- **Multi-Language**: Support for 100+ languages
- **RESTful API**: Simple, well-documented API
- **ACID Compliance**: Strong consistency guarantees

## Quick Start

### 1. Enable Typesense

Add to your `.env` file:

```bash
# Enable search with Typesense
SEARCH_ENABLED=true
SEARCH_PROVIDER=typesense
```

### 2. Build and Start

```bash
nself build
nself start
```

### 3. Verify Installation

```bash
nself search status
nself search test
```

## Configuration

### Basic Configuration

```bash
# Core settings
SEARCH_ENABLED=true
SEARCH_PROVIDER=typesense
SEARCH_PORT=8108
SEARCH_API_KEY=your-api-key-here  # Auto-generated if not set

# Route configuration
SEARCH_ROUTE=search  # Creates search.yourdomain.com

# Multi-tenancy
SEARCH_INDEX_PREFIX=tenant1_

# Language
SEARCH_LANGUAGE=en
```

### Advanced Configuration

```bash
# Typesense version
TYPESENSE_VERSION=27.1

# CORS settings
TYPESENSE_ENABLE_CORS=true

# Logging
TYPESENSE_LOG_LEVEL=info  # Options: debug, info, warn, error

# Performance tuning
TYPESENSE_NUM_MEMORY_SHARDS=4
TYPESENSE_SNAPSHOT_INTERVAL_SECONDS=3600
TYPESENSE_HEALTHY_READ_LAG=1000
TYPESENSE_HEALTHY_WRITE_LAG=500
```

### SSL/TLS (Production)

```bash
TYPESENSE_SSL_CERTIFICATE=/path/to/cert.pem
TYPESENSE_SSL_CERTIFICATE_KEY=/path/to/key.pem
```

## Usage

### JavaScript/TypeScript Client

#### Installation

```bash
npm install typesense
```

#### Basic Setup

```javascript
import Typesense from 'typesense'

const client = new Typesense.Client({
  nodes: [{
    host: 'localhost',
    port: '8108',
    protocol: 'http'
  }],
  apiKey: process.env.SEARCH_API_KEY,
  connectionTimeoutSeconds: 2
})
```

#### Create a Collection (Schema)

```javascript
const schema = {
  name: 'products',
  fields: [
    { name: 'name', type: 'string' },
    { name: 'description', type: 'string' },
    { name: 'price', type: 'float' },
    { name: 'category', type: 'string', facet: true },
    { name: 'rating', type: 'float', facet: true },
    { name: 'in_stock', type: 'bool', facet: true },
    { name: 'created_at', type: 'int64' }
  ],
  default_sorting_field: 'created_at'
}

await client.collections().create(schema)
```

#### Index Documents

```javascript
// Single document
const document = {
  name: 'MacBook Pro 16"',
  description: 'Powerful laptop for developers and creators',
  price: 2499.99,
  category: 'Laptops',
  rating: 4.8,
  in_stock: true,
  created_at: Date.now()
}

await client.collections('products').documents().create(document)

// Bulk import
const documents = [
  { name: 'Product 1', price: 100, category: 'Electronics' },
  { name: 'Product 2', price: 200, category: 'Electronics' },
  // ... more documents
]

await client.collections('products').documents().import(documents, {
  action: 'create'
})
```

#### Search

```javascript
// Basic search
const searchParameters = {
  q: 'laptop',
  query_by: 'name,description',
  limit: 10
}

const results = await client.collections('products')
  .documents()
  .search(searchParameters)

// Advanced search with filters and facets
const advancedSearch = {
  q: 'macbook',
  query_by: 'name,description',
  filter_by: 'price:<2000 && in_stock:true',
  facet_by: 'category,rating',
  sort_by: 'rating:desc,price:asc',
  limit: 20,
  page: 1,
  highlight_full_fields: 'name,description',
  typo_tokens_threshold: 2
}

const advancedResults = await client.collections('products')
  .documents()
  .search(advancedSearch)
```

#### Autocomplete

```javascript
const autocompleteParams = {
  q: 'mac',
  query_by: 'name',
  prefix: true,
  limit: 5
}

const suggestions = await client.collections('products')
  .documents()
  .search(autocompleteParams)
```

### Python Client

#### Installation

```bash
pip install typesense
```

#### Basic Setup

```python
import typesense
import os

client = typesense.Client({
    'nodes': [{
        'host': 'localhost',
        'port': '8108',
        'protocol': 'http'
    }],
    'api_key': os.environ['SEARCH_API_KEY'],
    'connection_timeout_seconds': 2
})
```

#### Create Collection

```python
schema = {
    'name': 'products',
    'fields': [
        {'name': 'name', 'type': 'string'},
        {'name': 'description', 'type': 'string'},
        {'name': 'price', 'type': 'float'},
        {'name': 'category', 'type': 'string', 'facet': True}
    ]
}

client.collections.create(schema)
```

#### Index and Search

```python
# Index document
document = {
    'name': 'MacBook Pro',
    'description': 'Powerful laptop',
    'price': 2499.99,
    'category': 'Laptops'
}

client.collections['products'].documents.create(document)

# Search
search_parameters = {
    'q': 'laptop',
    'query_by': 'name,description',
    'filter_by': 'price:<2000'
}

results = client.collections['products'].documents.search(search_parameters)
```

### REST API

#### Health Check

```bash
curl -H "X-TYPESENSE-API-KEY: ${SEARCH_API_KEY}" \
  http://localhost:8108/health
```

#### Create Collection

```bash
curl -X POST \
  -H "X-TYPESENSE-API-KEY: ${SEARCH_API_KEY}" \
  -H "Content-Type: application/json" \
  http://localhost:8108/collections \
  -d '{
    "name": "products",
    "fields": [
      {"name": "name", "type": "string"},
      {"name": "price", "type": "float"}
    ]
  }'
```

#### Index Document

```bash
curl -X POST \
  -H "X-TYPESENSE-API-KEY: ${SEARCH_API_KEY}" \
  -H "Content-Type: application/json" \
  http://localhost:8108/collections/products/documents \
  -d '{
    "name": "MacBook Pro",
    "price": 2499.99
  }'
```

#### Search

```bash
curl -H "X-TYPESENSE-API-KEY: ${SEARCH_API_KEY}" \
  "http://localhost:8108/collections/products/documents/search?q=laptop&query_by=name"
```

## Integration with Hasura

### 1. Create Hasura Action

Create a new action in Hasura Console:

```graphql
type Query {
  searchProducts(
    query: String!
    filters: String
    limit: Int
  ): SearchResult
}

type SearchResult {
  found: Int!
  hits: [Product]!
  facets: JSON
}

type Product {
  id: String!
  name: String!
  description: String
  price: Float!
  score: Float
}
```

### 2. Create Action Handler

```javascript
// functions/search-handler.js
import Typesense from 'typesense'

const client = new Typesense.Client({
  nodes: [{
    host: process.env.SEARCH_HOST || 'typesense',
    port: process.env.SEARCH_PORT || '8108',
    protocol: 'http'
  }],
  apiKey: process.env.SEARCH_API_KEY
})

export default async function handler(req, res) {
  const { query, filters, limit = 10 } = req.body.input

  try {
    const searchParameters = {
      q: query,
      query_by: 'name,description',
      limit
    }

    if (filters) {
      searchParameters.filter_by = filters
    }

    const results = await client.collections('products')
      .documents()
      .search(searchParameters)

    res.json({
      found: results.found,
      hits: results.hits.map(hit => ({
        id: hit.document.id,
        name: hit.document.name,
        description: hit.document.description,
        price: hit.document.price,
        score: hit.text_match
      })),
      facets: results.facet_counts
    })
  } catch (error) {
    res.status(500).json({ error: error.message })
  }
}
```

### 3. Sync Data from PostgreSQL

Use Hasura Event Triggers to sync data:

```javascript
// functions/sync-to-typesense.js
import Typesense from 'typesense'

const client = new Typesense.Client({
  nodes: [{
    host: process.env.SEARCH_HOST || 'typesense',
    port: process.env.SEARCH_PORT || '8108',
    protocol: 'http'
  }],
  apiKey: process.env.SEARCH_API_KEY
})

export default async function handler(req, res) {
  const { event, table } = req.body

  try {
    switch (event.op) {
      case 'INSERT':
      case 'UPDATE':
        await client.collections('products').documents().upsert(
          event.data.new
        )
        break

      case 'DELETE':
        await client.collections('products').documents(
          event.data.old.id
        ).delete()
        break
    }

    res.json({ success: true })
  } catch (error) {
    res.status(500).json({ error: error.message })
  }
}
```

## Advanced Features

### Geo Search

```javascript
// Create collection with geo field
const schema = {
  name: 'stores',
  fields: [
    { name: 'name', type: 'string' },
    { name: 'location', type: 'geopoint' }
  ]
}

await client.collections().create(schema)

// Index with coordinates
await client.collections('stores').documents().create({
  name: 'Store 1',
  location: [37.7749, -122.4194] // [lat, lng]
})

// Search with geo filter
const geoSearch = {
  q: '*',
  query_by: 'name',
  filter_by: 'location:(37.7749, -122.4194, 10 km)',
  sort_by: 'location(37.7749, -122.4194):asc'
}
```

### Vector Search (Semantic Search)

```javascript
// Create collection with vector field
const schema = {
  name: 'articles',
  fields: [
    { name: 'title', type: 'string' },
    { name: 'content', type: 'string' },
    { name: 'embedding', type: 'float[]', num_dim: 384 }
  ]
}

await client.collections().create(schema)

// Index with embeddings (from your ML model)
await client.collections('articles').documents().create({
  title: 'Article Title',
  content: 'Article content...',
  embedding: [0.1, 0.2, ...] // 384-dimensional vector
})

// Vector search
const vectorSearch = {
  q: '*',
  vector_query: 'embedding:([0.1, 0.2, ...], k:10)'
}
```

### Faceted Search

```javascript
const facetedSearch = {
  q: 'laptop',
  query_by: 'name,description',
  facet_by: 'category,brand,price_range',
  max_facet_values: 10
}

const results = await client.collections('products')
  .documents()
  .search(facetedSearch)

// Access facets
results.facet_counts.forEach(facet => {
  console.log(facet.field_name)
  facet.counts.forEach(count => {
    console.log(`  ${count.value}: ${count.count}`)
  })
})
```

## Performance Optimization

### 1. Use Specific Query Fields

```javascript
// Bad - searches all fields
{ q: 'laptop', query_by: '*' }

// Good - searches specific fields
{ q: 'laptop', query_by: 'name,description' }
```

### 2. Enable Prefix Search for Autocomplete

```javascript
{
  q: 'mac',
  query_by: 'name',
  prefix: true,  // Faster for autocomplete
  limit: 5
}
```

### 3. Use Filters Before Search

```javascript
{
  q: 'laptop',
  query_by: 'name,description',
  filter_by: 'in_stock:true && price:<2000',  // Filter first
  limit: 10
}
```

### 4. Batch Imports

```javascript
// Import in batches of 1000-10000
const batchSize = 5000
for (let i = 0; i < documents.length; i += batchSize) {
  const batch = documents.slice(i, i + batchSize)
  await client.collections('products').documents().import(batch, {
    action: 'upsert'
  })
}
```

### 5. Use Caching

Typesense has built-in caching. Enable it with:

```bash
TYPESENSE_CACHE_SIZE_MB=1000
```

## Monitoring

### Health Check

```bash
nself search health
```

### View Logs

```bash
nself search logs
nself search logs -f  # Follow logs
```

### Collection Stats

```bash
curl -H "X-TYPESENSE-API-KEY: ${SEARCH_API_KEY}" \
  http://localhost:8108/collections/products
```

### Server Stats

```bash
curl -H "X-TYPESENSE-API-KEY: ${SEARCH_API_KEY}" \
  http://localhost:8108/stats.json
```

## Backup and Restore

### Create Snapshot

```bash
curl -X POST \
  -H "X-TYPESENSE-API-KEY: ${SEARCH_API_KEY}" \
  http://localhost:8108/operations/snapshot?snapshot_path=/data/snapshot
```

### Export Collection

```bash
curl -H "X-TYPESENSE-API-KEY: ${SEARCH_API_KEY}" \
  "http://localhost:8108/collections/products/documents/export" \
  > products-export.jsonl
```

### Import Collection

```bash
curl -X POST \
  -H "X-TYPESENSE-API-KEY: ${SEARCH_API_KEY}" \
  -H "Content-Type: text/plain" \
  http://localhost:8108/collections/products/documents/import \
  --data-binary @products-export.jsonl
```

## Troubleshooting

### Service Not Starting

```bash
# Check logs
nself logs typesense

# Verify configuration
nself search status

# Check port availability
lsof -i :8108
```

### Performance Issues

```bash
# Check resource usage
docker stats ${PROJECT_NAME}_typesense

# Increase memory
# Edit docker-compose.yml
services:
  typesense:
    deploy:
      resources:
        limits:
          memory: 2G
```

### Search Not Returning Results

```bash
# Verify API key
curl -H "X-TYPESENSE-API-KEY: ${SEARCH_API_KEY}" \
  http://localhost:8108/health

# Check collection exists
curl -H "X-TYPESENSE-API-KEY: ${SEARCH_API_KEY}" \
  http://localhost:8108/collections

# Verify documents indexed
curl -H "X-TYPESENSE-API-KEY: ${SEARCH_API_KEY}" \
  http://localhost:8108/collections/products
```

## Comparison with MeiliSearch

| Feature | Typesense | MeiliSearch |
|---------|-----------|-------------|
| **Speed** | Sub-50ms | Very Fast |
| **Typo Tolerance** | ✅ Excellent | ✅ Excellent |
| **Geo Search** | ✅ Built-in | ❌ Not available |
| **Vector Search** | ✅ Built-in | ❌ Not available |
| **Resource Usage** | Low (200MB-1GB) | Medium (500MB-2GB) |
| **Multi-Tenancy** | ✅ Via API keys | ⚠️ Manual |
| **Dashboard** | ❌ No built-in UI | ✅ Beautiful UI |
| **Facets** | ✅ Excellent | ✅ Good |
| **Best For** | Instant search, autocomplete, semantic search | General purpose search, beautiful UI |

## When to Choose Typesense

Choose Typesense if you need:

- **Instant search** with sub-50ms latency
- **Geo-location search** with distance calculations
- **Vector/semantic search** for AI applications
- **Lower resource usage** than Elasticsearch
- **Strong consistency** guarantees
- **RESTful API** simplicity
- **Multi-language** support out of the box

## Resources

- [Typesense Documentation](https://typesense.org/docs/)
- [Typesense API Reference](https://typesense.org/docs/latest/api/)
- [Typesense GitHub](https://github.com/typesense/typesense)
- [Client Libraries](https://typesense.org/docs/latest/api/api-clients.html)
- [Typesense Cloud](https://cloud.typesense.org/)

## Related Documentation

- [Search Services](SEARCH.md) - General search documentation
- [Search Configuration](../configuration/ENVIRONMENT-VARIABLES.md) - Search environment variables
- [Services Overview](SERVICES.md) - All available services
