#!/usr/bin/env bash
# health-endpoints.sh - Comprehensive health endpoint templates
# Part of nself v0.9.8 - Production Features

# Generate health endpoint code for Node.js/Express services
generate_nodejs_health_endpoint() {

set -euo pipefail

  cat <<'EOF'
// =============================================================================
// PRODUCTION HEALTH ENDPOINTS
// =============================================================================

const os = require('os');
const { Pool } = require('pg');
const Redis = require('redis');

// Service metadata
const SERVICE_NAME = process.env.SERVICE_NAME || 'service';
const SERVICE_VERSION = process.env.SERVICE_VERSION || '1.0.0';
const START_TIME = Date.now();

// Database pool
let dbPool = null;
if (process.env.POSTGRES_HOST) {
  dbPool = new Pool({
    host: process.env.POSTGRES_HOST || 'postgres',
    port: 5432,
    database: process.env.POSTGRES_DB || 'nself',
    user: process.env.POSTGRES_USER || 'postgres',
    password: process.env.POSTGRES_PASSWORD || 'postgres',
    max: 10,
    idleTimeoutMillis: 30000,
  });
}

// Redis client
let redisClient = null;
if (process.env.REDIS_ENABLED === 'true') {
  redisClient = Redis.createClient({
    url: `redis://${process.env.REDIS_HOST || 'redis'}:6379`,
    password: process.env.REDIS_PASSWORD,
  });
  redisClient.on('error', (err) => console.error('Redis error:', err));
  redisClient.connect().catch(console.error);
}

/**
 * Liveness probe - Is the service alive?
 * Returns 200 if service can handle requests
 */
app.get('/health', (req, res) => {
  res.status(200).json({
    status: 'healthy',
    service: SERVICE_NAME,
    version: SERVICE_VERSION,
    timestamp: new Date().toISOString(),
  });
});

/**
 * Readiness probe - Is the service ready to accept traffic?
 * Checks all dependencies (database, redis, etc.)
 */
app.get('/ready', async (req, res) => {
  const checks = {
    service: 'up',
    database: 'not_configured',
    redis: 'not_configured',
  };

  let isReady = true;

  // Check database
  if (dbPool) {
    try {
      const result = await dbPool.query('SELECT 1');
      checks.database = 'healthy';
    } catch (error) {
      checks.database = 'unhealthy';
      isReady = false;
    }
  }

  // Check Redis
  if (redisClient) {
    try {
      await redisClient.ping();
      checks.redis = 'healthy';
    } catch (error) {
      checks.redis = 'unhealthy';
      isReady = false;
    }
  }

  const status = isReady ? 'ready' : 'not_ready';
  const statusCode = isReady ? 200 : 503;

  res.status(statusCode).json({
    status,
    service: SERVICE_NAME,
    version: SERVICE_VERSION,
    uptime_seconds: Math.floor((Date.now() - START_TIME) / 1000),
    checks,
    timestamp: new Date().toISOString(),
  });
});

/**
 * Detailed status endpoint - Full service diagnostics
 * Includes resource usage, dependencies, and metrics
 */
app.get('/status', async (req, res) => {
  const uptime = Math.floor((Date.now() - START_TIME) / 1000);
  const memUsage = process.memoryUsage();

  const status = {
    service: {
      name: SERVICE_NAME,
      version: SERVICE_VERSION,
      environment: process.env.NODE_ENV || 'development',
      uptime_seconds: uptime,
    },
    system: {
      platform: process.platform,
      node_version: process.version,
      hostname: os.hostname(),
      cpus: os.cpus().length,
      total_memory_mb: Math.round(os.totalmem() / 1024 / 1024),
      free_memory_mb: Math.round(os.freemem() / 1024 / 1024),
    },
    process: {
      pid: process.pid,
      memory: {
        rss_mb: Math.round(memUsage.rss / 1024 / 1024),
        heap_total_mb: Math.round(memUsage.heapTotal / 1024 / 1024),
        heap_used_mb: Math.round(memUsage.heapUsed / 1024 / 1024),
        external_mb: Math.round(memUsage.external / 1024 / 1024),
      },
      cpu_usage_percent: process.cpuUsage(),
    },
    dependencies: {
      database: 'not_configured',
      redis: 'not_configured',
    },
    timestamp: new Date().toISOString(),
  };

  // Check database connection
  if (dbPool) {
    try {
      const dbStart = Date.now();
      await dbPool.query('SELECT 1');
      const dbDuration = Date.now() - dbStart;
      status.dependencies.database = {
        status: 'healthy',
        response_time_ms: dbDuration,
        pool_total: dbPool.totalCount,
        pool_idle: dbPool.idleCount,
        pool_waiting: dbPool.waitingCount,
      };
    } catch (error) {
      status.dependencies.database = {
        status: 'unhealthy',
        error: error.message,
      };
    }
  }

  // Check Redis
  if (redisClient) {
    try {
      const redisStart = Date.now();
      await redisClient.ping();
      const redisDuration = Date.now() - redisStart;
      status.dependencies.redis = {
        status: 'healthy',
        response_time_ms: redisDuration,
      };
    } catch (error) {
      status.dependencies.redis = {
        status: 'unhealthy',
        error: error.message,
      };
    }
  }

  res.json(status);
});

EOF
}

# Generate health endpoint code for Python/Flask services
generate_python_health_endpoint() {
  cat <<'EOF'
# =============================================================================
# PRODUCTION HEALTH ENDPOINTS
# =============================================================================

import os
import time
import psutil
from datetime import datetime
from flask import Flask, jsonify

# Service metadata
SERVICE_NAME = os.getenv('SERVICE_NAME', 'service')
SERVICE_VERSION = os.getenv('SERVICE_VERSION', '1.0.0')
START_TIME = time.time()

@app.route('/health')
def health():
    """
    Liveness probe - Is the service alive?
    Returns 200 if service can handle requests
    """
    return jsonify({
        'status': 'healthy',
        'service': SERVICE_NAME,
        'version': SERVICE_VERSION,
        'timestamp': datetime.utcnow().isoformat()
    }), 200

@app.route('/ready')
def ready():
    """
    Readiness probe - Is the service ready to accept traffic?
    Checks all dependencies (database, redis, etc.)
    """
    checks = {
        'service': 'up',
        'database': 'not_configured',
        'redis': 'not_configured'
    }

    is_ready = True

    # Check database
    if db:
        try:
            with db.cursor() as cur:
                cur.execute('SELECT 1')
            checks['database'] = 'healthy'
        except Exception as e:
            checks['database'] = 'unhealthy'
            is_ready = False

    # Check Redis
    if redis_client:
        try:
            redis_client.ping()
            checks['redis'] = 'healthy'
        except Exception as e:
            checks['redis'] = 'unhealthy'
            is_ready = False

    status = 'ready' if is_ready else 'not_ready'
    status_code = 200 if is_ready else 503

    return jsonify({
        'status': status,
        'service': SERVICE_NAME,
        'version': SERVICE_VERSION,
        'uptime_seconds': int(time.time() - START_TIME),
        'checks': checks,
        'timestamp': datetime.utcnow().isoformat()
    }), status_code

@app.route('/status')
def status():
    """
    Detailed status endpoint - Full service diagnostics
    Includes resource usage, dependencies, and metrics
    """
    uptime = int(time.time() - START_TIME)
    process = psutil.Process()
    mem_info = process.memory_info()

    response = {
        'service': {
            'name': SERVICE_NAME,
            'version': SERVICE_VERSION,
            'environment': os.getenv('ENV', 'development'),
            'uptime_seconds': uptime
        },
        'system': {
            'platform': os.sys.platform,
            'python_version': os.sys.version,
            'hostname': os.uname().nodename,
            'cpus': os.cpu_count(),
            'total_memory_mb': int(psutil.virtual_memory().total / 1024 / 1024),
            'available_memory_mb': int(psutil.virtual_memory().available / 1024 / 1024)
        },
        'process': {
            'pid': os.getpid(),
            'memory': {
                'rss_mb': int(mem_info.rss / 1024 / 1024),
                'vms_mb': int(mem_info.vms / 1024 / 1024)
            },
            'cpu_percent': process.cpu_percent(interval=0.1),
            'threads': process.num_threads()
        },
        'dependencies': {
            'database': 'not_configured',
            'redis': 'not_configured'
        },
        'timestamp': datetime.utcnow().isoformat()
    }

    # Check database
    if db:
        try:
            db_start = time.time()
            with db.cursor() as cur:
                cur.execute('SELECT 1')
            db_duration = int((time.time() - db_start) * 1000)
            response['dependencies']['database'] = {
                'status': 'healthy',
                'response_time_ms': db_duration
            }
        except Exception as e:
            response['dependencies']['database'] = {
                'status': 'unhealthy',
                'error': str(e)
            }

    # Check Redis
    if redis_client:
        try:
            redis_start = time.time()
            redis_client.ping()
            redis_duration = int((time.time() - redis_start) * 1000)
            response['dependencies']['redis'] = {
                'status': 'healthy',
                'response_time_ms': redis_duration
            }
        except Exception as e:
            response['dependencies']['redis'] = {
                'status': 'unhealthy',
                'error': str(e)
            }

    return jsonify(response), 200

EOF
}

# Generate health endpoint code for Go services
generate_go_health_endpoint() {
  cat <<'EOF'
// =============================================================================
// PRODUCTION HEALTH ENDPOINTS
// =============================================================================

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/go-redis/redis/v8"
)

var (
	serviceName    = getEnv("SERVICE_NAME", "service")
	serviceVersion = getEnv("SERVICE_VERSION", "1.0.0")
	startTime      = time.Now()
)

type HealthResponse struct {
	Status    string    `json:"status"`
	Service   string    `json:"service"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
}

type ReadyResponse struct {
	Status        string            `json:"status"`
	Service       string            `json:"service"`
	Version       string            `json:"version"`
	UptimeSeconds int64             `json:"uptime_seconds"`
	Checks        map[string]string `json:"checks"`
	Timestamp     time.Time         `json:"timestamp"`
}

type StatusResponse struct {
	Service      ServiceInfo            `json:"service"`
	System       SystemInfo             `json:"system"`
	Process      ProcessInfo            `json:"process"`
	Dependencies map[string]interface{} `json:"dependencies"`
	Timestamp    time.Time              `json:"timestamp"`
}

type ServiceInfo struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	Environment    string `json:"environment"`
	UptimeSeconds  int64  `json:"uptime_seconds"`
}

type SystemInfo struct {
	Platform       string `json:"platform"`
	GoVersion      string `json:"go_version"`
	Hostname       string `json:"hostname"`
	CPUs           int    `json:"cpus"`
	TotalMemoryMB  uint64 `json:"total_memory_mb"`
}

type ProcessInfo struct {
	NumGoroutines int    `json:"num_goroutines"`
	AllocMB       uint64 `json:"alloc_mb"`
	TotalAllocMB  uint64 `json:"total_alloc_mb"`
	SysMB         uint64 `json:"sys_mb"`
}

// Health endpoint - Liveness probe
func healthHandler(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "healthy",
		Service:   serviceName,
		Version:   serviceVersion,
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Ready endpoint - Readiness probe
func readyHandler(db *sql.DB, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checks := map[string]string{
			"service":  "up",
			"database": "not_configured",
			"redis":    "not_configured",
		}

		isReady := true

		// Check database
		if db != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()

			if err := db.PingContext(ctx); err != nil {
				checks["database"] = "unhealthy"
				isReady = false
			} else {
				checks["database"] = "healthy"
			}
		}

		// Check Redis
		if rdb != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()

			if err := rdb.Ping(ctx).Err(); err != nil {
				checks["redis"] = "unhealthy"
				isReady = false
			} else {
				checks["redis"] = "healthy"
			}
		}

		status := "ready"
		statusCode := http.StatusOK
		if !isReady {
			status = "not_ready"
			statusCode = http.StatusServiceUnavailable
		}

		response := ReadyResponse{
			Status:        status,
			Service:       serviceName,
			Version:       serviceVersion,
			UptimeSeconds: int64(time.Since(startTime).Seconds()),
			Checks:        checks,
			Timestamp:     time.Now(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteStatus(statusCode)
		json.NewEncoder(w).Encode(response)
	}
}

// Status endpoint - Detailed diagnostics
func statusHandler(db *sql.DB, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		hostname, _ := os.Hostname()

		response := StatusResponse{
			Service: ServiceInfo{
				Name:          serviceName,
				Version:       serviceVersion,
				Environment:   getEnv("ENV", "development"),
				UptimeSeconds: int64(time.Since(startTime).Seconds()),
			},
			System: SystemInfo{
				Platform:  runtime.GOOS,
				GoVersion: runtime.Version(),
				Hostname:  hostname,
				CPUs:      runtime.NumCPU(),
			},
			Process: ProcessInfo{
				NumGoroutines: runtime.NumGoroutine(),
				AllocMB:       m.Alloc / 1024 / 1024,
				TotalAllocMB:  m.TotalAlloc / 1024 / 1024,
				SysMB:         m.Sys / 1024 / 1024,
			},
			Dependencies: map[string]interface{}{
				"database": "not_configured",
				"redis":    "not_configured",
			},
			Timestamp: time.Now(),
		}

		// Check database
		if db != nil {
			start := time.Now()
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()

			if err := db.PingContext(ctx); err != nil {
				response.Dependencies["database"] = map[string]interface{}{
					"status": "unhealthy",
					"error":  err.Error(),
				}
			} else {
				response.Dependencies["database"] = map[string]interface{}{
					"status":           "healthy",
					"response_time_ms": time.Since(start).Milliseconds(),
				}
			}
		}

		// Check Redis
		if rdb != nil {
			start := time.Now()
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()

			if err := rdb.Ping(ctx).Err(); err != nil {
				response.Dependencies["redis"] = map[string]interface{}{
					"status": "unhealthy",
					"error":  err.Error(),
				}
			} else {
				response.Dependencies["redis"] = map[string]interface{}{
					"status":           "healthy",
					"response_time_ms": time.Since(start).Milliseconds(),
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

EOF
}

# Export functions
export -f generate_nodejs_health_endpoint generate_python_health_endpoint generate_go_health_endpoint
