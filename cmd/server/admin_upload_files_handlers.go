package main

import (
	"database/sql"
	"net/http"

	"cups-web/internal/store"
)

func adminListUploadFilesHandler(w http.ResponseWriter, r *http.Request) {
	files, err := listUploadFiles(r, uploadDir)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to scan upload files")
		return
	}
	writeJSON(w, files)
}

func adminDeleteUploadFileHandler(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	if rel == "" {
		writeJSONError(w, http.StatusBadRequest, "missing upload path")
		return
	}
	deleted, err := removeUploadFile(uploadDir, rel)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "deleted": deleted})
}

func adminDeleteOrphanUploadFilesHandler(w http.ResponseWriter, r *http.Request) {
	files, err := listUploadFiles(r, uploadDir)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to scan upload files")
		return
	}
	deleted := 0
	for _, file := range files {
		if file.Linked {
			continue
		}
		ok, err := removeUploadFile(uploadDir, file.Path)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to delete orphan upload files")
			return
		}
		if ok {
			deleted++
		}
	}
	writeJSON(w, map[string]interface{}{"ok": true, "deleted": deleted})
}

func listUploadFiles(r *http.Request, uploads string) ([]uploadFileInfo, error) {
	linked := map[string][]int64{}
	err := appStore.WithTx(r.Context(), true, func(tx *sql.Tx) error {
		paths, err := store.ListPrintRecordPaths(r.Context(), tx, nil)
		if err != nil {
			return err
		}
		for _, path := range paths {
			linked[path.StoredPath] = append(linked[path.StoredPath], path.ID)
			converted := convertedRelPath(path.StoredPath)
			linked[converted] = append(linked[converted], path.ID)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return scanUploadFiles(uploads, linked)
}
