package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// providerRepository 供货商身份持久化,裸 SQL(表 provider_accounts,迁移 230),避开 ent codegen。
type providerRepository struct {
	db *sql.DB
}

// NewProviderRepository 构造供货商 repo。
func NewProviderRepository(db *sql.DB) service.ProviderRepository {
	return &providerRepository{db: db}
}

const providerSelectCols = `id, name, email, password_hash, allowed_group_ids, daily_add_limit, enabled, COALESCE(notes,''), last_login_at, created_at, updated_at`

func scanProvider(row interface{ Scan(...any) error }) (*service.ProviderAccount, error) {
	var (
		p        service.ProviderAccount
		groupIDs pq.Int64Array
		lastAt   sql.NullTime
	)
	if err := row.Scan(&p.ID, &p.Name, &p.Email, &p.PasswordHash, &groupIDs, &p.DailyAddLimit,
		&p.Enabled, &p.Notes, &lastAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.AllowedGroupIDs = []int64(groupIDs)
	if lastAt.Valid {
		p.LastLoginAt = &lastAt.Time
	}
	return &p, nil
}

func (r *providerRepository) Create(ctx context.Context, p *service.ProviderAccount) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO provider_accounts (name, email, password_hash, allowed_group_ids, daily_add_limit, enabled, notes)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		p.Name, p.Email, p.PasswordHash, pq.Array(p.AllowedGroupIDs), p.DailyAddLimit, p.Enabled, p.Notes,
	).Scan(&id)
	return id, err
}

func (r *providerRepository) GetByID(ctx context.Context, id int64) (*service.ProviderAccount, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+providerSelectCols+` FROM provider_accounts WHERE id=$1`, id)
	p, err := scanProvider(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrProviderNotFound
	}
	return p, err
}

func (r *providerRepository) GetByEmail(ctx context.Context, email string) (*service.ProviderAccount, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+providerSelectCols+` FROM provider_accounts WHERE lower(email)=lower($1)`, email)
	p, err := scanProvider(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrProviderNotFound
	}
	return p, err
}

func (r *providerRepository) List(ctx context.Context) ([]*service.ProviderAccount, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+providerSelectCols+` FROM provider_accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*service.ProviderAccount
	for rows.Next() {
		p, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *providerRepository) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE provider_accounts SET enabled=$1, updated_at=NOW() WHERE id=$2`, enabled, id)
	return err
}

func (r *providerRepository) UpdateLastLogin(ctx context.Context, id int64, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE provider_accounts SET last_login_at=$1, updated_at=NOW() WHERE id=$2`, at, id)
	return err
}

func (r *providerRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM provider_accounts WHERE id=$1`, id)
	return err
}

// CountAccountsAddedSince 统计该供货商名下、created_at >= since 的上游账号数(按 Extra.provider_id 标签)。
func (r *providerRepository) CountAccountsAddedSince(ctx context.Context, providerID int64, since time.Time) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE (extra->>'provider_id')::bigint = $1 AND created_at >= $2`,
		providerID, since,
	).Scan(&n)
	return n, err
}
