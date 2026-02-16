# ɳAdmin - Web Management Interface

ɳAdmin is the central web-based management interface for your entire ɳSelf deployment. It provides a unified dashboard to monitor, configure, and control all aspects of your Backend-as-a-Service infrastructure.

## Overview

ɳAdmin is a powerful, extensible administration panel that acts as your command center for:
- Real-time service monitoring and health checks
- Configuration management across all services
- Database administration (PostgreSQL, Redis)
- Log viewing and analysis
- Performance metrics and resource usage
- Service orchestration and scaling
- User and permission management

## Features

### Current Capabilities
- **Service Dashboard** - Real-time status of all running services
- **Configuration Editor** - Modify environment variables and settings
- **Health Monitoring** - Service health checks and uptime tracking
- **Log Viewer** - Centralized log access from all containers
- **Quick Actions** - Start/stop/restart services with one click
- **Resource Metrics** - CPU, memory, disk usage per service

### Planned Features
- **Database Management** - Built-in query editor and table browser (replacing pgAdmin)
- **User Management** - Create and manage authentication users
- **API Explorer** - Test GraphQL and REST endpoints
- **Backup Management** - Schedule and manage database backups
- **Migration Tools** - Database migration and seed management
- **Performance Profiler** - Identify bottlenecks and optimize queries
- **Alert Configuration** - Set up monitoring alerts and notifications
- **Template Manager** - Install and manage custom service templates
- **Security Auditor** - Security scanning and compliance checks

## Configuration

Enable ɳAdmin in your `.env` file:

```bash
# ɳAdmin Configuration
NSELF_ADMIN_ENABLED=true
NSELF_ADMIN_PORT=3021
NSELF_ADMIN_ROUTE=admin.${BASE_DOMAIN}

# Optional: Authentication
NSELF_ADMIN_AUTH_ENABLED=true
NSELF_ADMIN_USERNAME=admin
NSELF_ADMIN_PASSWORD=secure-password-here

# Optional: Advanced Settings
NSELF_ADMIN_THEME=dark
NSELF_ADMIN_LANGUAGE=en
NSELF_ADMIN_TIMEZONE=UTC
NSELF_ADMIN_SESSION_TIMEOUT=3600
```

## Access

After enabling and starting ɳAdmin:

### Local Development
- URL: `https://admin.local.nself.org`
- Default credentials: `admin` / `admin` (change immediately)

### Production
- URL: `https://admin.<your-domain>`
- Requires authentication setup

## Architecture

ɳAdmin is built with:
- **Frontend**: React with TypeScript, Material-UI
- **Backend**: Node.js with Express
- **Real-time**: WebSocket connections for live updates
- **Data Source**: Direct Docker API and service APIs

### Integration Points

ɳAdmin integrates with:
- **Docker API** - Container management and stats
- **PostgreSQL** - Direct database access
- **Hasura** - GraphQL schema introspection
- **Prometheus** - Metrics collection
- **Loki** - Log aggregation
- **Redis** - Cache and session inspection

## Use Cases

### 1. Development Environment Management
- Quickly reset databases
- View real-time logs during debugging
- Modify environment variables without restarting
- Test API endpoints

### 2. Production Monitoring
- Monitor service health and uptime
- Track resource usage trends
- Set up alerts for critical issues
- View aggregated logs

### 3. Database Administration
- Run SQL queries
- View and modify data
- Manage database users and permissions
- Export/import data

### 4. Service Orchestration
- Scale services up/down
- Rolling updates and deployments
- Service dependency management
- Load balancing configuration

## Security

ɳAdmin implements multiple security layers:
- **Authentication**: JWT-based authentication
- **Authorization**: Role-based access control (RBAC)
- **Encryption**: All traffic over HTTPS
- **Audit Logging**: All actions logged with user attribution
- **Session Management**: Automatic timeout and refresh
- **IP Whitelisting**: Optional IP-based access control

### Security Best Practices
1. Always change default credentials
2. Use strong passwords (minimum 16 characters)
3. Enable 2FA when available
4. Restrict access by IP in production
5. Regular security updates
6. Monitor audit logs

## Comparison with Other Admin Tools

### vs pgAdmin
- **Integrated**: Part of nself ecosystem, not standalone
- **Multi-service**: Manages all services, not just PostgreSQL
- **Lighter**: Lower resource usage
- **Unified Auth**: Single sign-on with nself Auth

### vs Portainer
- **Specialized**: Tailored for nself deployments
- **Simpler**: Focused UI without Docker complexity
- **Integrated Monitoring**: Built-in Prometheus/Grafana integration
- **nself-aware**: Understands nself service relationships

### vs Adminer
- **Modern UI**: React-based responsive interface
- **Multi-database**: PostgreSQL, Redis, and more
- **API Testing**: Includes GraphQL/REST testing
- **Real-time Updates**: WebSocket-based live data

## Customization

### Themes
ɳAdmin supports custom themes:
```javascript
// Custom theme example
{
  "primary": "#1976d2",
  "secondary": "#dc004e",
  "background": "#f5f5f5",
  "dark": true
}
```

### Plugins
Extend functionality with plugins:
```javascript
// Plugin structure
{
  "name": "custom-monitor",
  "version": "1.0.0",
  "hooks": {
    "dashboard": "renderCustomWidget",
    "menu": "addCustomMenuItem"
  }
}
```

### Custom Widgets
Add dashboard widgets for specific needs:
- Custom metrics displays
- Third-party service integration
- Business-specific KPIs
- Custom action buttons

## API

ɳAdmin exposes its own API for automation:

```bash
# Get service status
GET /api/services/status

# Restart a service
POST /api/services/{name}/restart

# Run database query
POST /api/database/query
{
  "query": "SELECT * FROM users LIMIT 10"
}

# Get logs
GET /api/logs/{service}?lines=100
```

## Troubleshooting

### Admin UI Not Loading
- Check `NSELF_ADMIN_ENABLED=true` in .env
- Verify port 3021 is not in use
- Check nginx routing configuration
- Ensure Docker socket is accessible

### Cannot Connect to Services
- Verify Docker network configuration
- Check service health endpoints
- Ensure proper environment variables
- Review firewall rules

### Authentication Issues
- Reset admin password via CLI: `nself admin reset-password`
- Check JWT secret configuration
- Verify session timeout settings
- Clear browser cache and cookies

## CLI Integration

Manage ɳAdmin from the command line:

```bash
# Enable admin UI
nself admin enable

# Disable admin UI
nself admin disable

# Reset admin password
nself admin reset-password

# View admin logs
nself admin logs

# Check admin status
nself admin status
```

## Development Mode (Admin-Dev)

**Added in v0.4.7**

For ɳAdmin contributors or those who want to run the admin UI locally with hot-reload while connecting to Docker backend services, nself provides an "admin-dev" mode.

### Overview

Admin-dev mode allows you to:
- Run ɳAdmin locally from source (e.g., `~/Sites/ɳAdmin`)
- Get hot-reload and debugging capabilities
- Connect to all Docker services (Postgres, Hasura, Auth, etc.)
- Access via the same `admin.local.nself.org` URL

### Quick Start

```bash
# Enable dev mode on port 3000
nself service admin dev enable 3000 ~/Sites/ɳAdmin

# Rebuild and restart to apply nginx routing changes
nself build && nself restart

# Start your local admin server
cd ~/Sites/ɳAdmin && npm run dev

# Access at https://admin.local.nself.org
```

### Commands

| Command | Description |
|---------|-------------|
| `nself service admin dev status` | Show current dev mode configuration |
| `nself service admin dev enable [port] [path]` | Enable dev mode |
| `nself service admin dev disable` | Disable dev mode (use Docker container) |
| `nself service admin dev env` | Show environment variables for local development |

### Environment Variables

When dev mode is enabled, these are added to your `.env`:

```bash
# Admin Development Mode (local dev server)
NSELF_ADMIN_DEV=true
NSELF_ADMIN_DEV_PORT=3000
NSELF_ADMIN_DEV_PATH=~/Sites/ɳAdmin  # Optional, for documentation
```

### Local Environment Setup

After enabling dev mode, get the required environment variables for your local admin:

```bash
nself service admin dev env
```

This outputs the environment variables to add to your local `ɳAdmin/.env.local`:

```bash
# Database (via Docker)
DATABASE_URL=postgres://postgres:password@localhost:5432/mydb

# Hasura (via Docker)
HASURA_GRAPHQL_ENDPOINT=http://localhost:8080/v1/graphql
HASURA_GRAPHQL_ADMIN_SECRET=your-hasura-secret

# Admin
ADMIN_SECRET_KEY=your-admin-secret
PROJECT_NAME=myproject
BASE_DOMAIN=local.nself.org
NODE_ENV=development

# Project path (your nself project)
PROJECT_PATH=/path/to/your/project
NSELF_PROJECT_PATH=/path/to/your/project
```

### How It Works

1. **Nginx Routing**: When `NSELF_ADMIN_DEV=true`, nginx routes `admin.*` to `host.docker.internal:PORT` instead of the Docker container
2. **Container Skip**: The ɳAdmin Docker container is not created during `nself build`
3. **Backend Services**: All other services (Postgres, Hasura, Auth, etc.) run normally in Docker
4. **Network Access**: Your local admin connects to Docker services via localhost ports

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Browser (https://admin.local.nself.org)                    │
└───────────────────────────┬─────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────┐
│  Nginx (Docker)                                             │
│  Routes admin.* → host.docker.internal:3000                 │
└───────────────────────────┬─────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────┐
│  Local ɳAdmin (localhost:3000)                         │
│  ~/Sites/ɳAdmin                                        │
│  npm run dev (hot-reload enabled)                           │
└───────────────────────────┬─────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────┐
│  Docker Services                                            │
│  ┌─────────┐ ┌────────┐ ┌──────┐ ┌───────┐                 │
│  │Postgres │ │ Hasura │ │ Auth │ │ Redis │ ...             │
│  │ :5432   │ │ :8080  │ │:4000 │ │ :6379 │                 │
│  └─────────┘ └────────┘ └──────┘ └───────┘                 │
└─────────────────────────────────────────────────────────────┘
```

### Switching Between Modes

```bash
# Switch to dev mode (local server)
nself service admin dev enable 3000
nself build && nself restart

# Switch back to Docker mode
nself service admin dev disable
nself build && nself restart
```

### Troubleshooting

#### Cannot connect to admin.local.nself.org
- Ensure your local server is running on the configured port
- Check that Docker services are running: `nself status`
- Verify nginx rebuilt: `nself build --force`

#### Local admin can't connect to Hasura/Postgres
- Verify Docker services are running: `nself status`
- Check ports are exposed: `docker ps`
- Ensure `.env.local` has correct connection strings

#### WebSocket errors in dev mode
- The nginx config includes WebSocket upgrade headers for hot-reload
- If issues persist, restart nginx: `docker restart <project>_nginx`

#### Linux servers
- On Linux (non-Docker Desktop), nginx routes to `172.17.0.1:PORT` instead of `host.docker.internal`
- This is automatically detected during build

## Resource Requirements

- **CPU**: 0.25 cores minimum
- **Memory**: 256MB minimum, 512MB recommended
- **Storage**: 100MB for application, 1GB for logs/metrics
- **Network**: Low bandwidth, increases with monitoring

## Future Roadmap

### Q1 2025
- Database query builder UI
- Advanced user management
- API documentation generator

### Q2 2025
- ML model management interface
- Kubernetes deployment support
- Multi-tenant administration

### Q3 2025
- Mobile app for monitoring
- AI-powered insights
- Automated optimization

## Related Documentation

- [Services Overview](SERVICES.md)
- [Optional Services](SERVICES_OPTIONAL.md)
- [Monitoring Bundle](MONITORING-BUNDLE.md)
- [Environment Configuration](../configuration/ENVIRONMENT-VARIABLES.md)
- [Troubleshooting](../guides/TROUBLESHOOTING.md)
