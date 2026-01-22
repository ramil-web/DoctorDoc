-- Таблица метаданных файлов
CREATE TABLE IF NOT EXISTS files (
                                     id            UUID PRIMARY KEY,
                                     original_name TEXT NOT NULL,
                                     file_path     TEXT NOT NULL,
                                     rows_count    INTEGER   DEFAULT 0,
                                     status        TEXT      DEFAULT 'uploaded',
                                     fingerprint   TEXT,
                                     is_downloaded BOOLEAN DEFAULT FALSE,
                                     stats         JSONB     DEFAULT '{}',
                                     errors        JSONB     DEFAULT '[]',
                                     created_at    TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Таблица старых лицензий (если используешь)
CREATE TABLE IF NOT EXISTS licenses (
                                        key_hash    TEXT PRIMARY KEY,
                                        tariff      TEXT NOT NULL,
                                        fingerprint TEXT,
                                        expires_at  TIMESTAMP NOT NULL,
                                        is_active   BOOLEAN   DEFAULT TRUE,
                                        created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Таблица подписок
CREATE TABLE IF NOT EXISTS subscriptions (
                                             id               SERIAL PRIMARY KEY,
                                             email            VARCHAR(255) UNIQUE,
    license_key      VARCHAR(50) UNIQUE,
    plan_type        VARCHAR(20), -- 'free', 'month', 'year'
    status           VARCHAR(20), -- 'active', 'expired'
    expires_at       TIMESTAMP,   -- дата окончания
    daily_mb_used    FLOAT DEFAULT 0,
    daily_files_used INT DEFAULT 0,
    last_action_date DATE DEFAULT CURRENT_DATE,
    ip_address       TEXT,
    fingerprint      TEXT
    );

-- НОВАЯ ТАБЛИЦА: Устройства привязанные к подписке (Лимит 5 устройств)
CREATE TABLE IF NOT EXISTS subscription_devices (
                                                    id              SERIAL PRIMARY KEY,
                                                    subscription_id INTEGER REFERENCES subscriptions(id) ON DELETE CASCADE,
    fingerprint     TEXT NOT NULL,
    added_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(subscription_id, fingerprint)
    );

-- Таблица для отслеживания использования (Free лимиты)
CREATE TABLE IF NOT EXISTS usage_stats (
                                           id          SERIAL PRIMARY KEY,
                                           fingerprint TEXT UNIQUE NOT NULL,
                                           ip_address  TEXT,
                                           count       INTEGER   DEFAULT 0,
                                           updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Индексы для быстрой проверки лимитов по IP и связки устройств
CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_identity ON usage_stats (ip_address, fingerprint);
CREATE INDEX IF NOT EXISTS idx_sub_devices_fp ON subscription_devices (fingerprint);

-- Гарантируем наличие полей в основной таблице подписок
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS ip_address TEXT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS fingerprint TEXT;