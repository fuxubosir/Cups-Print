package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestDeletePrintRecordOnlyDeletesOwnedRecord(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	userA := createTestUser(t, ctx, s, "user-a")
	userB := createTestUser(t, ctx, s, "user-b")
	recordA := createTestPrintRecord(t, ctx, s, userA.ID, "a.pdf")
	recordB := createTestPrintRecord(t, ctx, s, userB.ID, "b.pdf")

	err := s.WithTx(ctx, false, func(tx *sql.Tx) error {
		deleted, err := DeletePrintRecord(ctx, tx, recordB, userA.ID)
		if err != nil {
			return err
		}
		if deleted {
			t.Fatal("deleted another user's print record")
		}
		deleted, err = DeletePrintRecord(ctx, tx, recordA, userA.ID)
		if err != nil {
			return err
		}
		if !deleted {
			t.Fatal("did not delete owned print record")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	assertPrintRecordCount(t, ctx, s, userA.ID, 0)
	assertPrintRecordCount(t, ctx, s, userB.ID, 1)
}

func TestDeletePrintRecordsByUserIDOnlyClearsOwnedRecords(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	userA := createTestUser(t, ctx, s, "user-a")
	userB := createTestUser(t, ctx, s, "user-b")
	createTestPrintRecord(t, ctx, s, userA.ID, "a-1.pdf")
	createTestPrintRecord(t, ctx, s, userA.ID, "a-2.pdf")
	createTestPrintRecord(t, ctx, s, userB.ID, "b.pdf")

	err := s.WithTx(ctx, false, func(tx *sql.Tx) error {
		deleted, err := DeletePrintRecordsByUserID(ctx, tx, userA.ID)
		if err != nil {
			return err
		}
		if deleted != 2 {
			t.Fatalf("deleted %d records, want 2", deleted)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	assertPrintRecordCount(t, ctx, s, userA.ID, 0)
	assertPrintRecordCount(t, ctx, s, userB.ID, 1)
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func createTestUser(t *testing.T, ctx context.Context, s *Store, username string) User {
	t.Helper()
	var user User
	err := s.WithTx(ctx, false, func(tx *sql.Tx) error {
		var err error
		user, err = CreateUser(ctx, tx, CreateUserInput{
			Username:     username,
			PasswordHash: "test",
			Role:         RoleUser,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func createTestPrintRecord(t *testing.T, ctx context.Context, s *Store, userID int64, filename string) int64 {
	t.Helper()
	var id int64
	err := s.WithTx(ctx, false, func(tx *sql.Tx) error {
		var err error
		id, err = InsertPrintRecord(ctx, tx, &PrintRecord{
			UserID:     userID,
			PrinterURI: "ipp://printer",
			Filename:   filename,
			StoredPath: filename,
			Pages:      1,
			Status:     "printed",
			CreatedAt:  nowUTC(),
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertPrintRecordCount(t *testing.T, ctx context.Context, s *Store, userID int64, want int) {
	t.Helper()
	var got int
	err := s.WithTx(ctx, true, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM print_jobs WHERE user_id = ?", userID).Scan(&got)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("record count = %d, want %d", got, want)
	}
}
