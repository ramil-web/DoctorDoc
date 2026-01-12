package repository

import (
    "context"
    "database/sql"
    "doctordoc/internal/models"
    "encoding/json"
    "time"
)

type Repository interface {
    SaveFileMeta(ctx context.Context, meta models.FileMetadata) error
    GetFileMeta(ctx context.Context, id string) (*models.FileMetadata, error)
    CheckLicense(ctx context.Context, keyHash string) (*models.License, error)
    SaveLicense(ctx context.Context, l models.License) error
    IncrementUsage(ctx context.Context, fp string) (int, error)
    GetUsageCount(ctx context.Context, fp string) (int, error)
    CreateSubscription(email, plan string, duration time.Duration, code string) error
    CheckActiveSubscription(ip, fp string) (bool, error)
    GetDailyUsage(ip, fp string) (int, error)
}

type pgRepo struct {
    db *sql.DB
}

func NewRepository(db *sql.DB) Repository {
    return &pgRepo{db: db}
}

func (r *pgRepo) SaveFileMeta(ctx context.Context, meta models.FileMetadata) error {
    query := `INSERT INTO files (id, original_name, file_path, status, rows_count, stats, errors, created_at, fingerprint, is_downloaded)
              VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
              ON CONFLICT (id) DO UPDATE SET
                status = EXCLUDED.status,
                errors = EXCLUDED.errors,
                rows_count = EXCLUDED.rows_count,
                is_downloaded = EXCLUDED.is_downloaded,
                fingerprint = COALESCE(NULLIF(EXCLUDED.fingerprint, ''), files.fingerprint)`
                // ^ ЭТА СТРОКА: если EXCLUDED.fingerprint пустой, оставить тот, что уже в базе

    statsJSON, _ := json.Marshal(meta.Stats)
    errorsJSON, _ := json.Marshal(meta.Errors)

    _, err := r.db.ExecContext(ctx, query,
        meta.ID, meta.OriginalName, meta.FilePath, meta.Status,
        meta.RowsCount, statsJSON, errorsJSON, meta.CreatedAt,
        meta.Fingerprint, meta.IsDownloaded,
    )
    return err
}

func (r *pgRepo) GetFileMeta(ctx context.Context, id string) (*models.FileMetadata, error) {
    m := &models.FileMetadata{}
    var statsData, errorsData []byte

    // Добавлена колонка is_downloaded в SELECT
    query := `SELECT id, original_name, file_path, status, rows_count, stats, errors, created_at, fingerprint, is_downloaded FROM files WHERE id = $1`

    err := r.db.QueryRowContext(ctx, query, id).Scan(
        &m.ID,
        &m.OriginalName,
        &m.FilePath,
        &m.Status,
        &m.RowsCount,
        &statsData,
        &errorsData,
        &m.CreatedAt,
        &m.Fingerprint,
        &m.IsDownloaded, // Считываем флаг из базы
    )

    if err != nil {
        return nil, err
    }

    json.Unmarshal(statsData, &m.Stats)
    json.Unmarshal(errorsData, &m.Errors)
    return m, nil
}

func (r *pgRepo) CheckLicense(ctx context.Context, keyHash string) (*models.License, error) {
    var l models.License
    query := `SELECT key_hash, tariff, expires_at, is_active FROM licenses WHERE key_hash = $1`
    err := r.db.QueryRowContext(ctx, query, keyHash).Scan(&l.KeyHash, &l.Tariff, &l.ExpiresAt, &l.IsActive)
    if err != nil { return nil, err }
    return &l, nil
}
