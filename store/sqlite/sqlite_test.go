package sqlite_test

import (
	"path/filepath"
	"testing"

	"wharf/store/sqlite"
	"wharf/storetest"
)

func TestSQLiteConformance(t *testing.T) {
	st, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	storetest.Run(t, st)
}
