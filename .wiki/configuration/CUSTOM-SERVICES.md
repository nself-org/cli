# Custom Services Configuration

Add your own services alongside nself.

## Define Custom Services

In .env:
```bash
CS_1=my-api:express-js:8001
CS_2=my-worker:bullmq-js:8002
```

## Build

```bash
nself build
```

Generates service files in services/ directory.

## Deploy

```bash
nself start
nself logs my-api -f
```

See [Custom Services Guide](../guides/CUSTOM-SERVICES.md) for complete reference.

