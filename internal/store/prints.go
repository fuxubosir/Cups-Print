package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type PrintRecord struct {
	ID         int64
	UserID     int64
	Username   string
	PrinterURI string
	Filename   string
	StoredPath string
	Pages      int
	JobID      sql.NullString
	Status     string
	IsDuplex   bool
	IsColor    bool
	CreatedAt  string
}

type PrintFilter struct {
	Username string
	StartAt  string
	EndAt    string
	Limit    int
}

type PrintRecordPath struct {
	ID         int64
	StoredPath string
}

func InsertPrintRecord(ctx context.Context, tx *sql.Tx, rec *PrintRecord) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO print_jobs (
		user_id, printer_uri, filename, stored_path, pages,
		job_id, status, is_duplex, is_color, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.UserID, rec.PrinterURI, rec.Filename, rec.StoredPath, rec.Pages,
		rec.JobID, rec.Status, rec.IsDuplex, rec.IsColor, rec.CreatedAt,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func UpdatePrintStatus(ctx context.Context, tx *sql.Tx, id int64, status string, jobID string) error {
	_, err := tx.ExecContext(ctx, "UPDATE print_jobs SET status = ?, job_id = ? WHERE id = ?", status, jobID, id)
	return err
}

func DeletePrintRecord(ctx context.Context, tx *sql.Tx, id int64, userID int64) (bool, error) {
	res, err := tx.ExecContext(ctx, "DELETE FROM print_jobs WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

func DeletePrintRecordsByUserID(ctx context.Context, tx *sql.Tx, userID int64) (int64, error) {
	res, err := tx.ExecContext(ctx, "DELETE FROM print_jobs WHERE user_id = ?", userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func ListPrintRecordPaths(ctx context.Context, tx *sql.Tx, userID *int64) ([]PrintRecordPath, error) {
	query := "SELECT id, stored_path FROM print_jobs"
	args := []interface{}{}
	if userID != nil {
		query += " WHERE user_id = ?"
		args = append(args, *userID)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []PrintRecordPath
	for rows.Next() {
		var path PrintRecordPath
		if err := rows.Scan(&path.ID, &path.StoredPath); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

func GetPrintRecordByID(ctx context.Context, tx *sql.Tx, id int64) (PrintRecord, error) {
	row := tx.QueryRowContext(ctx, `SELECT
		p.id, p.user_id, u.username, p.printer_uri, p.filename, p.stored_path, p.pages,
		p.job_id, p.status, p.is_duplex, p.is_color, p.created_at
		FROM print_jobs p
		JOIN users u ON u.id = p.user_id
		WHERE p.id = ?`, id)
	var rec PrintRecord
	err := row.Scan(
		&rec.ID, &rec.UserID, &rec.Username, &rec.PrinterURI, &rec.Filename, &rec.StoredPath,
		&rec.Pages, &rec.JobID, &rec.Status, &rec.IsDuplex, &rec.IsColor, &rec.CreatedAt,
	)
	return rec, err
}

func ListPrintRecords(ctx context.Context, tx *sql.Tx, filter PrintFilter) ([]PrintRecord, error) {
	args := []interface{}{}
	conds := []string{"1=1"}
	if filter.Username != "" {
		conds = append(conds, "u.username = ?")
		args = append(args, filter.Username)
	}
	if filter.StartAt != "" {
		conds = append(conds, "p.created_at >= ?")
		args = append(args, filter.StartAt)
	}
	if filter.EndAt != "" {
		conds = append(conds, "p.created_at <= ?")
		args = append(args, filter.EndAt)
	}
	query := fmt.Sprintf(`SELECT
		p.id, p.user_id, u.username, p.printer_uri, p.filename, p.stored_path, p.pages,
		p.job_id, p.status, p.is_duplex, p.is_color, p.created_at
		FROM print_jobs p
		JOIN users u ON u.id = p.user_id
		WHERE %s
		ORDER BY p.created_at DESC`, strings.Join(conds, " AND "))
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []PrintRecord
	for rows.Next() {
		var rec PrintRecord
		if err := rows.Scan(
			&rec.ID, &rec.UserID, &rec.Username, &rec.PrinterURI, &rec.Filename, &rec.StoredPath,
			&rec.Pages, &rec.JobID, &rec.Status, &rec.IsDuplex, &rec.IsColor, &rec.CreatedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}
