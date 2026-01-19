package repository

import (
	"context"
	"database/sql"
	"doctordoc/internal/models"
	"time"
)

// IncrementUsage теперь принимает и IP
func (r *pgRepo) IncrementUsage(ctx context.Context, fp string, ip string) (int, error) {
    var count int
    // Теперь, когда fp одинаковый для Chrome и Firefox (hw_...),
    // этот запрос ВСЕГДА будет попадать в одну и ту же строку.
    query := `
        INSERT INTO usage_stats (fingerprint, ip_address, count, updated_at)
        VALUES ($1, $2, 1, NOW())
        ON CONFLICT (fingerprint) DO UPDATE SET
            count = CASE
                WHEN usage_stats.updated_at::date < CURRENT_DATE THEN 1
                ELSE usage_stats.count + 1
            END,
            ip_address = EXCLUDED.ip_address,
            updated_at = NOW()
        RETURNING count`

    err := r.db.QueryRowContext(ctx, query, fp, ip).Scan(&count)
    return count, err
}

// GetUsageCount теперь ищет по обоим параметрам
func (r *pgRepo) GetUsageCount(ctx context.Context, fp string, ip string) (int, error) {
    var count int
    // Считаем сумму по IP. Даже если в базе каким-то чудом
    // остались старые записи с разными FP, этот запрос их склеит.
    query := `
        SELECT COALESCE(SUM(count), 0)
        FROM usage_stats
        WHERE (ip_address = $1 OR fingerprint = $2)
        AND updated_at::date = CURRENT_DATE`

    err := r.db.QueryRowContext(ctx, query, ip, fp).Scan(&count)
    return count, err
}

func (r *pgRepo) CreateSubscription(email, plan string, duration time.Duration, code string) error {
	query := `INSERT INTO subscriptions (email, plan_type, status, expires_at, access_code)
              VALUES ($1, $2, 'active', $3, $4) ON CONFLICT (email) DO UPDATE SET expires_at = $3`
	_, err := r.db.Exec(query, email, plan, time.Now().Add(duration), code)
	return err
}

func (r *pgRepo) CheckActiveSubscription(ip, fp string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM subscriptions WHERE fingerprint = $1 AND status = 'active' AND expires_at > NOW())`
	err := r.db.QueryRow(query, fp).Scan(&exists)
	return exists, err
}

func (r *pgRepo) GetDailyUsage(ip, fp string) (int, error) {
	var count int
	query := `SELECT count FROM usage_stats WHERE fingerprint = $1 AND updated_at >= CURRENT_DATE`
	err := r.db.QueryRow(query, fp).Scan(&count)
	if err == sql.ErrNoRows { return 0, nil }
	return count, err
}

func (r *pgRepo) SaveLicense(ctx context.Context, l models.License) error {
	query := `INSERT INTO licenses (key_hash, tariff, expires_at, is_active) VALUES ($1, $2, $3, $4) ON CONFLICT (key_hash) DO UPDATE SET is_active=$4`
	_, err := r.db.ExecContext(ctx, query, l.KeyHash, l.Tariff, l.ExpiresAt, l.IsActive)
	return err
}