package main

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fileHash(t *testing.T, p string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func TestBackupNameValidation(t *testing.T) {
	setupTestStore(t)
	bad := []string{"../activity.json", `..\activity.json`, "a/b.json", "notes.json", "activity-x.txt", "", "activity-../x.json"}
	for _, n := range bad {
		if _, err := backupPath(n); err == nil {
			t.Errorf("backupPath(%q) accepted, want refused", n)
		}
	}
	if _, err := backupPath("activity-daily-2026-11-16.json"); err != nil {
		t.Errorf("valid name refused: %v", err)
	}
}

func TestRestoreRequiresTypedWord(t *testing.T) {
	setupTestStore(t)
	code, _ := restoreBackup("activity-daily-2026-11-16.json", "restore")
	if code != http.StatusBadRequest {
		t.Fatalf("restore without RESTORE = %d, want 400", code)
	}
}

func TestPreviewRejectsBrokenFile(t *testing.T) {
	setupTestStore(t)
	os.MkdirAll(filepath.Join(dataDir, "Backups"), 0700)
	p := filepath.Join(dataDir, "Backups", "activity-broken.json")
	os.WriteFile(p, []byte("{not json"), 0600)
	code, body := backupPreview("activity-broken.json")
	if code != http.StatusUnprocessableEntity || body["parses"] != false {
		t.Fatalf("broken preview = %d %v", code, body)
	}
}

// TestRestoreKeepsCurrentDataAndBackup is the data-safety test:
// the current data is saved as a NEW backup first, the chosen backup is not changed,
// and the restored totals equal the backup's totals.
func TestRestoreKeepsCurrentDataAndBackup(t *testing.T) {
	setupTestStore(t)
	record("key", "KeyA", 0)
	record("key", "KeyB", 0)
	storeMu.Lock()
	rollMinuteLocked(time.Now(), true) // fold the minute so the snapshot file holds A and B
	storeMu.Unlock()
	if err := saveStore(); err != nil {
		t.Fatal(err)
	}
	chosen, err := backupNow("test-snapshot")
	if err != nil {
		t.Fatal(err)
	}
	chosenName := filepath.Base(chosen)
	chosenHash := fileHash(t, chosen)

	// Add more data after the snapshot, so current data differs from the backup.
	// These five are deliberately NOT saved to the file: the restore must save them first.
	for i := 0; i < 5; i++ {
		record("key", "KeyC", 0)
	}
	currentKeys := totalKeysNow()
	if currentKeys != 7 {
		t.Fatalf("setup: current keys = %d, want 7", currentKeys)
	}

	code, body := restoreBackup(chosenName, "RESTORE")
	if code != http.StatusOK {
		t.Fatalf("restore = %d %v", code, body)
	}
	if got := totalKeysNow(); got != 2 {
		t.Fatalf("restored keys = %d, want 2", got)
	}
	beforeName, _ := body["before_restore"].(string)
	if beforeName == "" {
		t.Fatalf("no before_restore name in %v", body)
	}
	beforePath := filepath.Join(dataDir, "Backups", beforeName)
	var saved Store
	b, _ := os.ReadFile(beforePath)
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatalf("before-restore copy unreadable: %v", err)
	}
	if k, _ := totalsOf(&saved); k["keys"] != uint64(7) {
		t.Fatalf("before-restore copy keys = %v, want 7", k["keys"])
	}
	if fileHash(t, chosen) != chosenHash {
		t.Fatalf("the chosen backup file was changed by restore")
	}
}

// totalKeysNow sums keys over all days in the live store, plus the running minute.
func totalKeysNow() uint64 {
	storeMu.RLock()
	defer storeMu.RUnlock()
	n := cur.keys
	for _, d := range store.Days {
		n += d.Keys
	}
	return n
}
