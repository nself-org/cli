# Database Migrations Guide

Version control your database schema with migrations.

## Overview

Migrations track schema changes:
- Create tables
- Add columns
- Modify indexes
- Run any SQL

## Creating Migrations

```bash
nself db migrate create "add_posts_table"
```

Edit the migration file and add SQL.

## Running Migrations

```bash
nself db migrate
```

## Rollback

```bash
nself db migrate rollback
```

See [Database Guide](DATABASE-WORKFLOW.md) for complete reference.

