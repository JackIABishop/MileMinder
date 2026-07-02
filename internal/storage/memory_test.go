package storage_test

import (
	"testing"

	"github.com/jackiabishop/mileminder/internal/storage"
	"github.com/jackiabishop/mileminder/internal/storage/storagetest"
)

func TestMemoryConformance(t *testing.T) {
	storagetest.RunConformance(t, func(t *testing.T) storage.Store {
		return storage.NewMemory()
	})
}

// A scoped store from MemoryTenants must itself satisfy the full Store contract.
func TestMemoryTenantsConformance(t *testing.T) {
	storagetest.RunConformance(t, func(t *testing.T) storage.Store {
		return storage.NewMemoryTenants().ForUser("user-1")
	})
}

func TestMemoryTenantsIsolation(t *testing.T) {
	storagetest.RunTenantIsolation(t, func(t *testing.T) storage.Tenants {
		return storage.NewMemoryTenants()
	})
}
