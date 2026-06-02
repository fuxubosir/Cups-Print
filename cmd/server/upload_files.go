package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type uploadFileInfo struct {
	Path      string  `json:"path"`
	Size      int64   `json:"size"`
	Modified  string  `json:"modified"`
	Linked    bool    `json:"linked"`
	RecordIDs []int64 `json:"recordIds"`
}

func resolveUploadPath(baseDir string, rel string) (string, error) {
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if cleanRel == "." || filepath.IsAbs(cleanRel) {
		return "", errors.New("invalid upload path")
	}
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Join(baseAbs, cleanRel))
	if err != nil {
		return "", err
	}
	within, err := filepath.Rel(baseAbs, pathAbs)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", errors.New("upload path outside base directory")
	}
	return pathAbs, nil
}

func removeUploadFile(baseDir string, rel string) (bool, error) {
	abs, err := resolveUploadPath(baseDir, rel)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("upload path is not a regular file")
	}
	if err := os.Remove(abs); err != nil {
		return false, err
	}
	removeEmptyUploadDirs(baseDir, filepath.Dir(abs))
	return true, nil
}

func removeStoredFiles(baseDir string, storedRel string) (int, error) {
	removed := 0
	for _, rel := range []string{storedRel, convertedRelPath(storedRel)} {
		ok, err := removeUploadFile(baseDir, rel)
		if err != nil {
			return removed, err
		}
		if ok {
			removed++
		}
	}
	return removed, nil
}

func removeEmptyUploadDirs(baseDir string, dir string) {
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return
	}
	for {
		dirAbs, err := filepath.Abs(dir)
		if err != nil || dirAbs == baseAbs {
			return
		}
		within, err := filepath.Rel(baseAbs, dirAbs)
		if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
			return
		}
		if err := os.Remove(dirAbs); err != nil {
			return
		}
		dir = filepath.Dir(dirAbs)
	}
}

func scanUploadFiles(baseDir string, linked map[string][]int64) ([]uploadFileInfo, error) {
	files := []uploadFileInfo{}
	err := filepath.WalkDir(baseDir, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		recordIDs := linked[rel]
		files = append(files, uploadFileInfo{
			Path:      rel,
			Size:      info.Size(),
			Modified:  info.ModTime().UTC().Format(time.RFC3339),
			Linked:    len(recordIDs) > 0,
			RecordIDs: recordIDs,
		})
		return nil
	})
	return files, err
}
