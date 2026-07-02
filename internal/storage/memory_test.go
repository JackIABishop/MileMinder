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
