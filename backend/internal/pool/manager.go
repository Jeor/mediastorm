package pool

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/javi11/nntppool"
)

// Manager provides centralized NNTP connection pool management
type Manager interface {
	// GetPool returns the current connection pool or error if not available
	GetPool() (nntppool.UsenetConnectionPool, error)

	// SetProviders creates/recreates the pool with new providers
	SetProviders(providers []nntppool.UsenetProviderConfig) error

	// ClearPool shuts down and removes the current pool
	ClearPool() error

	// HasPool returns true if a pool is currently available
	HasPool() bool
}

// manager implements the Manager interface
type manager struct {
	mu         sync.RWMutex
	pool       nntppool.UsenetConnectionPool
	generation uint64
	newPool    func(nntppool.Config) (nntppool.UsenetConnectionPool, error)
}

// NewManager creates a new pool manager
func NewManager() Manager {
	return &manager{newPool: newConnectionPool}
}

func newConnectionPool(config nntppool.Config) (nntppool.UsenetConnectionPool, error) {
	return nntppool.NewConnectionPool(config)
}

// GetPool returns the current connection pool or error if not available
func (m *manager) GetPool() (nntppool.UsenetConnectionPool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.pool == nil {
		return nil, fmt.Errorf("NNTP connection pool not available - no providers configured")
	}

	return m.pool, nil
}

// SetProviders creates/recreates the pool with new providers
func (m *manager) SetProviders(providers []nntppool.UsenetProviderConfig) error {
	m.mu.Lock()
	m.generation++
	generation := m.generation
	oldPool := m.pool
	m.pool = nil
	m.mu.Unlock()

	// Pool shutdown can itself wait on upstream background work, so do not make
	// settings saves wait for it.
	if oldPool != nil {
		slog.Info("Shutting down existing NNTP connection pool")
		go oldPool.Quit()
	}

	// Return early if no providers (clear pool scenario)
	if len(providers) == 0 {
		slog.Info("No NNTP providers configured - pool cleared")
		return nil
	}

	// nntppool verifies providers synchronously with a hardcoded 60-second
	// timeout. Build in the background so startup and settings saves return
	// immediately. Copy the slice because the caller retains ownership of it.
	providerSnapshot := append([]nntppool.UsenetProviderConfig(nil), providers...)
	go m.buildPool(generation, providerSnapshot)
	return nil
}

func (m *manager) buildPool(generation uint64, providers []nntppool.UsenetProviderConfig) {
	slog.Info("Creating NNTP connection pool in background", "provider_count", len(providers))
	pool, err := m.newPool(nntppool.Config{
		Providers:      providers,
		Logger:         slog.Default(),
		DelayType:      nntppool.DelayTypeFixed,
		RetryDelay:     10 * time.Millisecond,
		MinConnections: 2, // Keep 2 warm connections per provider for faster STAT commands
	})
	if err != nil {
		slog.Error("Failed to create NNTP connection pool", "error", err)
		return
	}

	m.mu.Lock()
	if generation != m.generation {
		m.mu.Unlock()
		slog.Info("Discarding superseded NNTP connection pool")
		pool.Quit()
		return
	}
	m.pool = pool
	m.mu.Unlock()
	slog.Info("NNTP connection pool created successfully")
}

// ClearPool shuts down and removes the current pool
func (m *manager) ClearPool() error {
	m.mu.Lock()
	m.generation++
	oldPool := m.pool
	m.pool = nil
	m.mu.Unlock()

	if oldPool != nil {
		slog.Info("Clearing NNTP connection pool")
		go oldPool.Quit()
	}

	return nil
}

// HasPool returns true if a pool is currently available
func (m *manager) HasPool() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.pool != nil
}
