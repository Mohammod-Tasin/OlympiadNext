package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"olympiadnext/internal/domain/notice"
)

const noticeColumns = `id, text_en, text_bn, display_order, is_active, created_at, updated_at`

type NoticeRepository struct {
	db *sql.DB
}

func NewNoticeRepository(db *sql.DB) *NoticeRepository {
	return &NoticeRepository{db: db}
}

func (r *NoticeRepository) Create(ctx context.Context, n *notice.Notice) error {
	const q = `
		INSERT INTO notices (id, text_en, text_bn, display_order, is_active, created_at, updated_at)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, now(), now())
		RETURNING id, created_at, updated_at`

	err := r.db.QueryRowContext(ctx, q, n.TextEN, n.TextBN, n.DisplayOrder, n.IsActive).
		Scan(&n.ID, &n.CreatedAt, &n.UpdatedAt)
	if err != nil {
		return fmt.Errorf("notice_repository: create failed: %w", err)
	}
	return nil
}

func (r *NoticeRepository) Update(ctx context.Context, n *notice.Notice) error {
	const q = `
		UPDATE notices
		SET text_en = $1, text_bn = $2, display_order = $3, is_active = $4, updated_at = now()
		WHERE id = $5
		RETURNING updated_at`

	err := r.db.QueryRowContext(ctx, q, n.TextEN, n.TextBN, n.DisplayOrder, n.IsActive, n.ID).
		Scan(&n.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return notice.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("notice_repository: update failed: %w", err)
	}
	return nil
}

func (r *NoticeRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM notices WHERE id = $1`

	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("notice_repository: delete failed: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("notice_repository: delete rows affected failed: %w", err)
	}
	if n == 0 {
		return notice.ErrNotFound
	}
	return nil
}

func (r *NoticeRepository) ListAll(ctx context.Context) ([]*notice.Notice, error) {
	const q = `SELECT ` + noticeColumns + ` FROM notices ORDER BY display_order ASC, created_at ASC`
	return r.list(ctx, q)
}

func (r *NoticeRepository) ListActive(ctx context.Context) ([]*notice.Notice, error) {
	const q = `SELECT ` + noticeColumns + ` FROM notices WHERE is_active ORDER BY display_order ASC, created_at ASC`
	return r.list(ctx, q)
}

func (r *NoticeRepository) list(ctx context.Context, q string) ([]*notice.Notice, error) {
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("notice_repository: list failed: %w", err)
	}
	defer rows.Close()

	var out []*notice.Notice
	for rows.Next() {
		var n notice.Notice
		if err := rows.Scan(&n.ID, &n.TextEN, &n.TextBN, &n.DisplayOrder, &n.IsActive, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("notice_repository: scan failed: %w", err)
		}
		out = append(out, &n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notice_repository: list iteration failed: %w", err)
	}
	return out, nil
}
