package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveUploadPathRejectsOutsidePath(t *testing.T) {
	baseDir := t.TempDir()
	if _, err := resolveUploadPath(baseDir, "../outside.txt"); err == nil {
		t.Fatal("accepted path outside upload directory")
	}
	if _, err := resolveUploadPath(baseDir, ""); err == nil {
		t.Fatal("accepted empty path")
	}
}

func TestRemoveStoredFilesRemovesSourceAndConvertedFile(t *testing.T) {
	baseDir := t.TempDir()
	storedRel := "20260602/document.pdf"
	writeUploadTestFile(t, baseDir, storedRel)
	writeUploadTestFile(t, baseDir, convertedRelPath(storedRel))

	removed, err := removeStoredFiles(baseDir, storedRel)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed %d files, want 2", removed)
	}
	for _, rel := range []string{storedRel, convertedRelPath(storedRel)} {
		abs, err := resolveUploadPath(baseDir, rel)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(abs); !os.IsNotExist(err) {
			t.Fatalf("file %q still exists", rel)
		}
	}
}

func TestScanUploadFilesMarksLinkedFiles(t *testing.T) {
	baseDir := t.TempDir()
	writeUploadTestFile(t, baseDir, "20260602/linked.pdf")
	writeUploadTestFile(t, baseDir, "20260602/orphan.pdf")

	files, err := scanUploadFiles(baseDir, map[string][]int64{
		"20260602/linked.pdf": {42},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("scanned %d files, want 2", len(files))
	}
	linked := map[string]bool{}
	for _, file := range files {
		linked[file.Path] = file.Linked
	}
	if !linked["20260602/linked.pdf"] {
		t.Fatal("linked file was marked orphaned")
	}
	if linked["20260602/orphan.pdf"] {
		t.Fatal("orphaned file was marked linked")
	}
}

func writeUploadTestFile(t *testing.T, baseDir string, rel string) {
	t.Helper()
	abs, err := resolveUploadPath(baseDir, rel)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
}
