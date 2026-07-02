package yamlstore_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackiabishop/mileminder/internal/storage"
	"github.com/jackiabishop/mileminder/internal/storage/storagetest"
	"github.com/jackiabishop/mileminder/internal/storage/yamlstore"
)

// A scoped store from Tenants must itself satisfy the full Store contract.
func TestYAMLTenantsConformance(t *testing.T) {
	storagetest.RunConformance(t, func(t *testing.T) storage.Store {
		return yamlstore.NewTenants(t.TempDir()).ForUser("user-1")
	})
}

func TestYAMLTenantsIsolation(t *testing.T) {
	storagetest.RunTenantIsolation(t, func(t *testing.T) storage.Tenants {
		return yamlstore.NewTenants(t.TempDir())
	})
}

// A malformed user id must resolve to a Store that fails every operation, so a
// traversal-shaped id can never touch a directory outside the users root.
func TestYAMLTenantsRejectsMalformedUserID(t *testing.T) {
	tn := yamlstore.NewTenants(t.TempDir())
	ctx := context.Background()
	bad := []string{"", "..", "../escape", "a/b", "foo.bar", strings.Repeat("x", 200)}
	for _, id := range bad {
		st := tn.ForUser(id)
		if err := st.SaveVehicle(ctx, "golf", sample()); err == nil {
			t.Fatalf("malformed id %q: SaveVehicle should fail", id)
		}
		if _, err := st.GetVehicle(ctx, "golf"); err == nil {
			t.Fatalf("malformed id %q: GetVehicle should fail", id)
		}
	}
}
