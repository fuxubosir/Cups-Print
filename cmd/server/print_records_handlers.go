package main

import (
	"database/sql"
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"cups-web/internal/auth"
	"cups-web/internal/store"

	"github.com/gorilla/mux"
)

type printRecordResponse struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"userId"`
	Username   string `json:"username"`
	PrinterURI string `json:"printerUri"`
	Filename   string `json:"filename"`
	Pages      int    `json:"pages"`
	JobID      string `json:"jobId"`
	Status     string `json:"status"`
	IsDuplex   bool   `json:"isDuplex"`
	IsColor    bool   `json:"isColor"`
	CreatedAt  string `json:"createdAt"`
}

func printRecordsHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	startAt, endAt, err := parseDateRange(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid date range")
		return
	}

	var resp []printRecordResponse
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		user, err := store.GetUserByID(r.Context(), tx, sess.UserID)
		if err != nil {
			return err
		}
		records, err := store.ListPrintRecords(r.Context(), tx, store.PrintFilter{
			Username: user.Username,
			StartAt:  startAt,
			EndAt:    endAt,
		})
		if err != nil {
			return err
		}
		resp = mapPrintRecords(records)
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load records")
		return
	}
	writeJSON(w, resp)
}

func deletePrintRecordHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid record id")
		return
	}

	var deleted bool
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		var err error
		deleted, err = store.DeletePrintRecord(r.Context(), tx, id, sess.UserID)
		return err
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to delete record")
		return
	}
	if !deleted {
		writeJSONError(w, http.StatusNotFound, "record not found")
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func clearPrintRecordsHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var deleted int64
	err = appStore.WithTx(r.Context(), false, func(tx *sql.Tx) error {
		var err error
		deleted, err = store.DeletePrintRecordsByUserID(r.Context(), tx, sess.UserID)
		return err
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to clear records")
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "deleted": deleted})
}

func adminPrintRecordsHandler(w http.ResponseWriter, r *http.Request) {
	startAt, endAt, err := parseDateRange(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid date range")
		return
	}
	username := r.URL.Query().Get("username")

	var resp []printRecordResponse
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		records, err := store.ListPrintRecords(r.Context(), tx, store.PrintFilter{
			Username: username,
			StartAt:  startAt,
			EndAt:    endAt,
		})
		if err != nil {
			return err
		}
		resp = mapPrintRecords(records)
		return nil
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load records")
		return
	}
	writeJSON(w, resp)
}

func printRecordFileHandler(w http.ResponseWriter, r *http.Request) {
	sess, err := auth.GetSession(r)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid record id")
		return
	}

	var record store.PrintRecord
	err = appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		rec, err := store.GetPrintRecordByID(r.Context(), tx, id)
		if err != nil {
			return err
		}
		record = rec
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "record not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to load record")
		return
	}
	if sess.Role != store.RoleAdmin && record.UserID != sess.UserID {
		writeJSONError(w, http.StatusForbidden, "forbidden")
		return
	}

	absPath := filepath.Join(uploadDir, filepath.FromSlash(record.StoredPath))
	f, err := os.Open(absPath)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "file not found")
		return
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to stat file")
		return
	}

	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": record.Filename})
	w.Header().Set("Content-Disposition", disposition)
	http.ServeContent(w, r, record.Filename, stat.ModTime(), f)
}

func parseDateRange(r *http.Request) (string, string, error) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" && end == "" {
		return "", "", nil
	}
	var startAt string
	var endAt string
	if start != "" {
		t, err := time.ParseInLocation("2006-01-02", start, time.Local)
		if err != nil {
			return "", "", err
		}
		startAt = t.UTC().Format(time.RFC3339)
	}
	if end != "" {
		t, err := time.ParseInLocation("2006-01-02", end, time.Local)
		if err != nil {
			return "", "", err
		}
		t = t.AddDate(0, 0, 1).Add(-time.Second)
		endAt = t.UTC().Format(time.RFC3339)
	}
	return startAt, endAt, nil
}

func mapPrintRecords(records []store.PrintRecord) []printRecordResponse {
	resp := make([]printRecordResponse, 0, len(records))
	for _, rec := range records {
		jobID := ""
		if rec.JobID.Valid {
			jobID = rec.JobID.String
		}
		resp = append(resp, printRecordResponse{
			ID:         rec.ID,
			UserID:     rec.UserID,
			Username:   rec.Username,
			PrinterURI: rec.PrinterURI,
			Filename:   rec.Filename,
			Pages:      rec.Pages,
			JobID:      jobID,
			Status:     rec.Status,
			IsDuplex:   rec.IsDuplex,
			IsColor:    rec.IsColor,
			CreatedAt:  rec.CreatedAt,
		})
	}
	return resp
}
