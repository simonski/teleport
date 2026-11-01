package toolhistory

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentAppendPreservesOneHeaderAndAllRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.csv")
	var workers sync.WaitGroup
	for n := range 32 {
		workers.Go(func() {
			row := []string{fmt.Sprint(n), "fixture", "start", "finish", "0.1", "success", "0", ""}
			if err := appendRecord(path, row); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil || len(rows) != 33 {
		t.Fatalf("rows=%d error=%v", len(rows), err)
	}
	if fmt.Sprint(rows[0]) != fmt.Sprint(fields) {
		t.Fatalf("header=%v", rows[0])
	}
	seen := map[string]bool{}
	for _, row := range rows[1:] {
		if seen[row[0]] {
			t.Fatalf("duplicate invocation %s", row[0])
		}
		seen[row[0]] = true
	}
}
