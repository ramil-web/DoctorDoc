package repository

import (
    "context"
    "database/sql"
    "doctordoc/internal/models"
    "errors"
    "time"
)

func (r *pgRepo) IncrementUsage(ctx context.Context, fp string, ip string) (int, error) {
    var count int
    query := `
        INSERT INTO usage_stats (fingerprint, ip_address, count, updated_at)
        VALUES ($1, $2, 1, NOW())
        ON CONFLICT (fingerprint) DO UPDATE SET
            count = CASE
                WHEN usage_stats.updated_at < NOW() - INTERVAL '24 hours' THEN 1
                ELSE usage_stats.count + 1
            END,
            ip_address = EXCLUDED.ip_address,
            updated_at = NOW()
        RETURNING count`

    err := r.db.QueryRowContext(ctx, query, fp, ip).Scan(&count)
    return count, err
}

func (r *pgRepo) GetUsageCount(ctx context.Context, fp string, ip string) (int, error) {
    var count int
    query := `
        SELECT COALESCE(SUM(count), 0)
        FROM usage_stats
        WHERE (ip_address = $1 OR fingerprint = $2)
        AND updated_at >= NOW() - INTERVAL '24 hours'`

    err := r.db.QueryRowContext(ctx, query, ip, fp).Scan(&count)
    return count, err
}

func (r *pgRepo) GetUsageWithTime(ctx context.Context, fp string, ip string) (int, time.Time, error) {
    var count int
    var updatedAt time.Time
    query := `SELECT count, updated_at FROM usage_stats WHERE fingerprint = $1 OR ip_address = $2 ORDER BY updated_at DESC LIMIT 1`
    err := r.db.QueryRowContext(ctx, query, fp, ip).Scan(&count, &updatedAt)
    if err != nil {
        return 0, time.Time{}, err
    }
    return count, updatedAt, nil
}

func (r *pgRepo) GetDistinctDevicesCount(ctx context.Context, ip string) (int, error) {
    var count int
    query := `SELECT COUNT(DISTINCT fingerprint) FROM usage_stats WHERE ip_address = $1 AND updated_at >= NOW() - INTERVAL '24 hours'`
    err := r.db.QueryRowContext(ctx, query, ip).Scan(&count)
    return count, err
}

func (r *pgRepo) CreateSubscription(email, plan string, duration time.Duration, code string) error {
    query := `INSERT INTO subscriptions (email, plan_type, status, expires_at, access_code)
              VALUES ($1, $2, 'active', $3, $4) ON CONFLICT (email) DO UPDATE SET expires_at = $3`
    _, err := r.db.Exec(query, email, plan, time.Now().Add(duration), code)
    return err
}

// Новая логика для 5 устройств
func (r *pgRepo) ActivateLicense(ctx context.Context, key string, fp string) (models.Subscription, error) {
    var sub models.Subscription
    var subID int // локальный ID для связей в БД

    // Используем твой PlanType
    query := `SELECT id, email, plan_type, expires_at FROM subscriptions WHERE access_code = $1 AND expires_at > NOW()`
    err := r.db.QueryRowContext(ctx, query, key).Scan(&subID, &sub.Email, &sub.PlanType, &sub.ExpiresAt)
    if err != nil { return sub, err }

    var exists bool
    r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM subscription_devices WHERE subscription_id = $1 AND fingerprint = $2)`, subID, fp).Scan(&exists)
    if exists { return sub, nil }

    var count int
    r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscription_devices WHERE subscription_id = $1`, subID).Scan(&count)
    if count >= 5 { return sub, errors.New("limit_exceeded") }

    _, err = r.db.ExecContext(ctx, `INSERT INTO subscription_devices (subscription_id, fingerprint, added_at) VALUES ($1, $2, NOW())`, subID, fp)
    return sub, err
}

func (r *pgRepo) CheckActiveSubscription(ip, fp string) (bool, error) {
    var exists bool
    // Проверка по таблице привязанных устройств (fingerprint)
    query := `SELECT EXISTS(SELECT 1 FROM subscription_devices sd JOIN subscriptions s ON sd.subscription_id = s.id
              WHERE sd.fingerprint = $1 AND s.expires_at > NOW())`
    err := r.db.QueryRow(query, fp).Scan(&exists)
    return exists, err
}

func (r *pgRepo) GetDailyUsage(ip, fp string) (int, error) {
    var count int
    query := `SELECT count FROM usage_stats WHERE fingerprint = $1 AND updated_at >= NOW() - INTERVAL '24 hours'`
    err := r.db.QueryRow(query, fp).Scan(&count)
    if err == sql.ErrNoRows { return 0, nil }
    return count, err
}

func (r *pgRepo) SaveLicense(ctx context.Context, l models.License) error {
    query := `INSERT INTO licenses (key_hash, tariff, expires_at, is_active) VALUES ($1, $2, $3, $4) ON CONFLICT (key_hash) DO UPDATE SET is_active=$4`
    _, err := r.db.ExecContext(ctx, query, l.KeyHash, l.Tariff, l.ExpiresAt, l.IsActive)
    return err
}