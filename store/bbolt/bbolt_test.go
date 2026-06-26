package bbolt_test

import (
	"path/filepath"
	"testing"

	"wharf/store/bbolt"
	"wharf/storetest"
)

func TestBboltConformance(t *testing.T) {
	st, err := bbolt.Open(filepath.Join(t.TempDir(), "test.boltdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	storetest.Run(t, st)
}
