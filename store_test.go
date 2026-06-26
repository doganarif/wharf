package wharf_test

import (
	"testing"

	"wharf"
	"wharf/storetest"
)

func TestMemoryStore(t *testing.T) {
	storetest.Run(t, wharf.NewMemoryStore())
}
