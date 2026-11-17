package main

// F9 backup preview and restore (v2.0.0). Preview only reads. Restore first saves the
// current statistics file as a new backup, then replaces the live data. No backup file is
// ever deleted or renamed by this code.

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var backupNameRE = regexp.MustCompile(`^activity-[A-Za-z0-9._-]+\.json$`)

// backupPath returns the full path of a backup inside the Backups folder. Anything that is
// not a plain file name (path separators, "..") is refused.
func backupPath(name string) (string, error) {
	if !backupNameRE.MatchString(name) || filepath.Base(name) != name {
		return "", errors.New("not a backup file name")
	}
	return filepath.Join(dataDir, "Backups", name), nil
}

// totalsOf adds up the data of a store (all days).
func totalsOf(s *Store) (map[string]any, map[string]string) {
	var keys, clicks, scroll, words, active uint64
	from, to := "", ""
	for k, d := range s.Days {
		keys += d.Keys
		clicks += d.Clicks
		scroll += d.Scrolls
		words += d.Words
		active += uint64(d.ActiveMin)
		if from == "" || k < from {
			from = k
		}
		if to == "" || k > to {
			to = k
		}
	}
	return map[string]any{"keys": keys, "clicks": clicks, "scroll": scroll, "words": words, "active_min": active},
		map[string]string{"from": from, "to": to}
}

// readBackup loads and parses one backup. The returned status is an HTTP status code.
func readBackup(name string) (*Store, int64, int, error) {
	p, err := backupPath(name)
	if err != nil {
		return nil, 0, http.StatusBadRequest, err
	}
	fi, err := os.Stat(p)
	if err != nil {
		return nil, 0, http.StatusNotFound, errors.New("backup not found")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, fi.Size(), http.StatusInternalServerError, err
	}
	var s Store
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fi.Size(), http.StatusUnprocessableEntity, errors.New("this backup cannot be read")
	}
	s.normalize()
	return &s, fi.Size(), http.StatusOK, nil
}

// backupPreview is the body of GET /api/backups/preview?name=...
func backupPreview(name string) (int, map[string]any) {
	s, size, status, err := readBackup(name)
	if err != nil {
		return status, map[string]any{"name": name, "bytes": size, "parses": false, "error": err.Error()}
	}
	totals, span := totalsOf(s)
	return http.StatusOK, map[string]any{
		"name": name, "bytes": size, "parses": true, "store_version": s.Version,
		"date_range": span, "totals": totals,
	}
}

// restoreBackup is the body of POST /api/backups/restore. Body: {"name": "...", "confirm": "RESTORE"}.
func restoreBackup(name, confirm string) (int, map[string]any) {
	if confirm != "RESTORE" {
		return http.StatusBadRequest, map[string]any{"error": "Type RESTORE exactly to confirm. Nothing was changed."}
	}
	s, _, status, err := readBackup(name)
	if err != nil {
		return status, map[string]any{"error": err.Error() + ". Nothing was changed."}
	}
	if loadFailed {
		return http.StatusConflict, map[string]any{"error": "The statistics file could not be read when the app started, so it is not replaced. Restart the app first. Nothing was changed."}
	}
	// Fold the running minute into its day first (as the app does at exit), then save, so the
	// copy below holds everything recorded so far. Without this the counts of the current
	// minute are in memory only and would be lost when the data is replaced.
	storeMu.Lock()
	rollMinuteLocked(time.Now(), true)
	storeMu.Unlock()
	if err := saveStore(); err != nil {
		return http.StatusInternalServerError, map[string]any{"error": "Could not save the current data first, so nothing was restored: " + err.Error()}
	}
	// Save the current data as a new backup before anything is replaced.
	before, err := backupNow("before-restore")
	if err != nil {
		return http.StatusInternalServerError, map[string]any{"error": "Could not save the current data first, so nothing was restored: " + err.Error()}
	}
	s.Version = storeVersion
	s.AppVersion = appVersion
	storeMu.Lock()
	store = *s
	lastRecorded = time.Time{}
	cur = minuteAcc{start: time.Now().Truncate(time.Minute)}
	storeMu.Unlock()
	dirty.Store(true)
	if err := saveStore(); err != nil {
		return http.StatusInternalServerError, map[string]any{"error": "Restored in memory but could not save: " + err.Error(), "before_restore": filepath.Base(before)}
	}
	totals, _ := totalsOf(s)
	logLine("restored from " + name + "; previous data saved as " + before)
	return http.StatusOK, map[string]any{"restored": true, "before_restore": filepath.Base(before), "totals": totals}
}

// writeJSONStatus writes a JSON body with an explicit HTTP status.
func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
