#!/usr/bin/env bash
# graceful-shutdown.sh - Graceful shutdown handler templates
# Part of nself v0.9.8 - Production Features

# Generate graceful shutdown for Node.js/Express services
generate_nodejs_shutdown() {

set -euo pipefail

  cat <<'EOF'
// =============================================================================
// GRACEFUL SHUTDOWN HANDLER
// =============================================================================

const SHUTDOWN_TIMEOUT_MS = 30000; // 30 seconds

// Track active connections
let activeConnections = new Set();
let isShuttingDown = false;

// Middleware to track connections
app.use((req, res, next) => {
  if (isShuttingDown) {
    res.set('Connection', 'close');
    return res.status(503).json({
      error: 'Service shutting down',
      message: 'Server is gracefully shutting down. Please retry your request.'
    });
  }

  const connectionId = `${Date.now()}-${Math.random()}`;
  activeConnections.add(connectionId);

  res.on('finish', () => {
    activeConnections.delete(connectionId);
  });

  next();
});

// Graceful shutdown function
async function gracefulShutdown(signal) {
  console.log(`\n${signal} received. Starting graceful shutdown...`);
  isShuttingDown = true;

  const shutdownStart = Date.now();

  // Stop accepting new connections
  server.close(() => {
    console.log('HTTP server closed to new connections');
  });

  // Wait for active connections to finish
  const checkInterval = setInterval(() => {
    const elapsed = Date.now() - shutdownStart;
    const remaining = activeConnections.size;

    console.log(`Active connections: ${remaining}, Elapsed: ${elapsed}ms`);

    if (remaining === 0) {
      clearInterval(checkInterval);
      performCleanup();
    } else if (elapsed > SHUTDOWN_TIMEOUT_MS) {
      clearInterval(checkInterval);
      console.warn(`Shutdown timeout reached. Forcing shutdown with ${remaining} active connections.`);
      performCleanup();
    }
  }, 1000);
}

// Cleanup resources
async function performCleanup() {
  console.log('Performing cleanup...');

  // Close database connections
  if (dbPool) {
    try {
      await dbPool.end();
      console.log('Database pool closed');
    } catch (error) {
      console.error('Error closing database pool:', error);
    }
  }

  // Close Redis connection
  if (redisClient) {
    try {
      await redisClient.quit();
      console.log('Redis connection closed');
    } catch (error) {
      console.error('Error closing Redis connection:', error);
    }
  }

  // Flush any buffered logs
  if (typeof logger !== 'undefined' && logger.flush) {
    await logger.flush();
  }

  console.log('Cleanup complete. Exiting.');
  process.exit(0);
}

// Handle shutdown signals
process.on('SIGTERM', () => gracefulShutdown('SIGTERM'));
process.on('SIGINT', () => gracefulShutdown('SIGINT'));

// Handle uncaught errors
process.on('uncaughtException', (error) => {
  console.error('Uncaught Exception:', error);
  gracefulShutdown('UNCAUGHT_EXCEPTION');
});

process.on('unhandledRejection', (reason, promise) => {
  console.error('Unhandled Rejection at:', promise, 'reason:', reason);
  gracefulShutdown('UNHANDLED_REJECTION');
});

EOF
}

# Generate graceful shutdown for Python/Flask services
generate_python_shutdown() {
  cat <<'EOF'
# =============================================================================
# GRACEFUL SHUTDOWN HANDLER
# =============================================================================

import signal
import sys
import time
import threading
from flask import request, g

SHUTDOWN_TIMEOUT = 30  # seconds
active_requests = []
is_shutting_down = False
shutdown_lock = threading.Lock()

# Middleware to track active requests
@app.before_request
def track_request():
    global is_shutting_down

    if is_shutting_down:
        return jsonify({
            'error': 'Service shutting down',
            'message': 'Server is gracefully shutting down. Please retry your request.'
        }), 503

    request_id = f"{time.time()}-{id(request)}"
    with shutdown_lock:
        active_requests.append(request_id)
    g.request_id = request_id

@app.after_request
def untrack_request(response):
    if hasattr(g, 'request_id'):
        with shutdown_lock:
            try:
                active_requests.remove(g.request_id)
            except ValueError:
                pass
    return response

def graceful_shutdown(signum, frame):
    """Handle graceful shutdown on SIGTERM or SIGINT"""
    global is_shutting_down

    signal_name = 'SIGTERM' if signum == signal.SIGTERM else 'SIGINT'
    print(f'\n{signal_name} received. Starting graceful shutdown...')

    is_shutting_down = True
    shutdown_start = time.time()

    # Wait for active requests to complete
    while True:
        with shutdown_lock:
            remaining = len(active_requests)

        elapsed = time.time() - shutdown_start

        if remaining == 0:
            print('All requests completed. Proceeding with cleanup.')
            break
        elif elapsed > SHUTDOWN_TIMEOUT:
            print(f'Shutdown timeout reached. Forcing shutdown with {remaining} active requests.')
            break
        else:
            print(f'Waiting for {remaining} active requests... ({elapsed:.1f}s elapsed)')
            time.sleep(1)

    perform_cleanup()

def perform_cleanup():
    """Clean up resources before exit"""
    print('Performing cleanup...')

    # Close database connection
    if db:
        try:
            db.close()
            print('Database connection closed')
        except Exception as e:
            print(f'Error closing database: {e}')

    # Close Redis connection
    if redis_client:
        try:
            redis_client.close()
            print('Redis connection closed')
        except Exception as e:
            print(f'Error closing Redis: {e}')

    # Flush logs
    import logging
    logging.shutdown()

    print('Cleanup complete. Exiting.')
    sys.exit(0)

# Register signal handlers
signal.signal(signal.SIGTERM, graceful_shutdown)
signal.signal(signal.SIGINT, graceful_shutdown)

EOF
}

# Generate graceful shutdown for Go services
generate_go_shutdown() {
  cat <<'EOF'
// =============================================================================
// GRACEFUL SHUTDOWN HANDLER
// =============================================================================

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

const shutdownTimeout = 30 * time.Second

var (
	activeRequests sync.WaitGroup
	isShuttingDown bool
	shutdownMutex  sync.RWMutex
)

// Middleware to track active requests
func trackRequestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shutdownMutex.RLock()
		shuttingDown := isShuttingDown
		shutdownMutex.RUnlock()

		if shuttingDown {
			w.Header().Set("Connection", "close")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"error":   "Service shutting down",
				"message": "Server is gracefully shutting down. Please retry your request.",
			})
			return
		}

		activeRequests.Add(1)
		defer activeRequests.Done()

		next.ServeHTTP(w, r)
	})
}

// Setup graceful shutdown
func setupGracefulShutdown(server *http.Server, db *sql.DB, rdb *redis.Client) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("\n%s received. Starting graceful shutdown...", sig)

		// Mark as shutting down
		shutdownMutex.Lock()
		isShuttingDown = true
		shutdownMutex.Unlock()

		// Create shutdown context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		// Shutdown HTTP server (stops accepting new connections)
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
		log.Println("HTTP server stopped accepting new connections")

		// Wait for active requests with timeout
		done := make(chan struct{})
		go func() {
			activeRequests.Wait()
			close(done)
		}()

		select {
		case <-done:
			log.Println("All active requests completed")
		case <-ctx.Done():
			log.Println("Shutdown timeout reached, forcing shutdown")
		}

		// Cleanup resources
		performCleanup(db, rdb)
	}()
}

// Cleanup resources
func performCleanup(db *sql.DB, rdb *redis.Client) {
	log.Println("Performing cleanup...")

	// Close database
	if db != nil {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		} else {
			log.Println("Database connection closed")
		}
	}

	// Close Redis
	if rdb != nil {
		if err := rdb.Close(); err != nil {
			log.Printf("Error closing Redis: %v", err)
		} else {
			log.Println("Redis connection closed")
		}
	}

	log.Println("Cleanup complete. Exiting.")
	os.Exit(0)
}

// Main setup
func main() {
	// ... existing setup code ...

	// Wrap handler with tracking middleware
	handler := trackRequestMiddleware(mux)

	// Create server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: handler,
	}

	// Setup graceful shutdown
	setupGracefulShutdown(server, db, rdb)

	// Start server
	log.Printf("Server starting on port %s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

EOF
}

# Export functions
export -f generate_nodejs_shutdown generate_python_shutdown generate_go_shutdown
