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

// ListOwnedAccounts 列出该供货商(Extra.provider_id)名下账号实况。
func (r *providerRepository) ListOwnedAccounts(ctx context.Context, providerID int64) ([]*service.ProviderOwnedAccount, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, COALESCE(name,''), COALESCE(platform,''), COALESCE(type,''), COALESCE(status,''),
		        COALESCE(error_message,''), last_used_at, rate_limited_at, rate_limit_reset_at, expires_at, created_at
		 FROM accounts WHERE (extra->>'provider_id')::bigint = $1 ORDER BY id DESC`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*service.ProviderOwnedAccount
	for rows.Next() {
		var (
			a                                              service.ProviderOwnedAccount
			lastUsed, rlAt, rlReset, expires               sql.NullTime
		)
		if err := rows.Scan(&a.ID, &a.Name, &a.Platform, &a.Type, &a.Status, &a.ErrorMessage,
			&lastUsed, &rlAt, &rlReset, &expires, &a.CreatedAt); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			a.LastUsedAt = &lastUsed.Time
		}
		if rlAt.Valid {
			a.RateLimitedAt = &rlAt.Time
		}
		if rlReset.Valid {
			a.RateLimitResetAt = &rlReset.Time
		}
		if expires.Valid {
			a.ExpiresAt = &expires.Time
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// AccountOwnedBy 校验账号是否属于该供货商(越权防护)。
func (r *providerRepository) AccountOwnedBy(ctx context.Context, providerID, accountID int64) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE id=$1 AND (extra->>'provider_id')::bigint = $2`,
		accountID, providerID,
	).Scan(&n)
	return n > 0, err
}

// AccountUsageSince 汇总该账号自 since 起的 token/请求数(仅 token,不含金额)。
func (r *providerRepository) AccountUsageSince(ctx context.Context, providerID, accountID int64, since time.Time) (*service.ProviderAccountUsage, error) {
	var u service.ProviderAccountUsage
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)
		 FROM usage_logs WHERE account_id=$1 AND created_at >= $2`,
		accountID, since,
	).Scan(&u.Requests, &u.InputTokens, &u.OutputTokens)
	return &u, err
}

// ---- v2 作用域板块实现 ----

func (r *providerRepository) Dashboard(ctx context.Context, providerID int64) (*service.ProviderDashboard, error) {
	d := &service.ProviderDashboard{ByStatus: map[string]int{}}
	// 账号总数 + 按状态
	rows, err := r.db.QueryContext(ctx,
		`SELECT COALESCE(status,''), COUNT(*) FROM accounts WHERE (extra->>'provider_id')::bigint=$1 GROUP BY status`, providerID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			rows.Close()
			return nil, err
		}
		d.ByStatus[st] = n
		d.TotalAccounts += n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// 今日用量(仅 token/请求数)
	since := time.Now().Truncate(24 * time.Hour)
	_ = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(u.input_tokens),0), COALESCE(SUM(u.output_tokens),0)
		 FROM usage_logs u JOIN accounts a ON a.id=u.account_id
		 WHERE (a.extra->>'provider_id')::bigint=$1 AND u.created_at >= $2`,
		providerID, since,
	).Scan(&d.TodayRequests, &d.TodayInputTokens, &d.TodayOutputTokens)
	// 代理数
	_ = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM proxies WHERE provider_id=$1 AND deleted_at IS NULL`, providerID).Scan(&d.ProxyCount)
	return d, nil
}

// ListGroups 只读:按 id 集合返回分组标识(仅 id/name/platform/status,零用户信息)。
func (r *providerRepository) ListGroups(ctx context.Context, ids []int64) ([]*service.ProviderGroupView, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, COALESCE(name,''), COALESCE(platform,''), COALESCE(status,'') FROM groups WHERE id = ANY($1) ORDER BY id`,
		pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*service.ProviderGroupView
	for rows.Next() {
		var g service.ProviderGroupView
		if err := rows.Scan(&g.ID, &g.Name, &g.Platform, &g.Status); err != nil {
			return nil, err
		}
		out = append(out, &g)
	}
	return out, rows.Err()
}

func (r *providerRepository) CreateProxy(ctx context.Context, providerID int64, p *service.ProviderProxy, username, password string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO proxies (name, protocol, host, port, username, password, status, provider_id, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,'active',$7,NOW(),NOW()) RETURNING id`,
		p.Name, p.Protocol, p.Host, p.Port, username, password, providerID,
	).Scan(&id)
	return id, err
}

func (r *providerRepository) ListProxies(ctx context.Context, providerID int64) ([]*service.ProviderProxy, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, COALESCE(name,''), COALESCE(protocol,''), COALESCE(host,''), COALESCE(port,0), COALESCE(status,''), created_at
		 FROM proxies WHERE provider_id=$1 AND deleted_at IS NULL ORDER BY id DESC`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*service.ProviderProxy
	for rows.Next() {
		var p service.ProviderProxy
		if err := rows.Scan(&p.ID, &p.Name, &p.Protocol, &p.Host, &p.Port, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// DeleteProxy 软删除,仅限本供货商名下代理。
func (r *providerRepository) DeleteProxy(ctx context.Context, providerID, proxyID int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE proxies SET deleted_at=NOW(), updated_at=NOW() WHERE id=$1 AND provider_id=$2 AND deleted_at IS NULL`,
		proxyID, providerID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("proxy not found or not owned")
	}
	return nil
}

// Usage 按账号聚合(仅 token/请求数,JOIN accounts 但不出任何 user 字段)。
func (r *providerRepository) Usage(ctx context.Context, providerID int64, since time.Time) ([]*service.ProviderUsageRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT u.account_id, COALESCE(a.name,''), COUNT(*), COALESCE(SUM(u.input_tokens),0), COALESCE(SUM(u.output_tokens),0)
		 FROM usage_logs u JOIN accounts a ON a.id=u.account_id
		 WHERE (a.extra->>'provider_id')::bigint=$1 AND u.created_at >= $2
		 GROUP BY u.account_id, a.name ORDER BY COUNT(*) DESC`,
		providerID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*service.ProviderUsageRow
	for rows.Next() {
		var r0 service.ProviderUsageRow
		if err := rows.Scan(&r0.AccountID, &r0.AccountName, &r0.Requests, &r0.InputTokens, &r0.OutputTokens); err != nil {
			return nil, err
		}
		out = append(out, &r0)
	}
	return out, rows.Err()
}

func (r *providerRepository) InsertAudit(ctx context.Context, providerID int64, action, detail, ip string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO provider_audit_logs (provider_id, action, detail, client_ip) VALUES ($1,$2,$3,$4)`,
		providerID, action, detail, ip)
	return err
}

func (r *providerRepository) ListAudit(ctx context.Context, providerID int64, limit int) ([]*service.ProviderAuditRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, action, COALESCE(detail,''), COALESCE(client_ip,''), created_at
		 FROM provider_audit_logs WHERE provider_id=$1 ORDER BY created_at DESC LIMIT $2`,
		providerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*service.ProviderAuditRow
	for rows.Next() {
		var a service.ProviderAuditRow
		if err := rows.Scan(&a.ID, &a.Action, &a.Detail, &a.ClientIP, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}
