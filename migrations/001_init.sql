
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

CREATE TABLE IF NOT EXISTS licenses (
                                        key_hash    TEXT PRIMARY KEY,
                                        tariff      TEXT NOT NULL,
                                        fingerprint TEXT,
                                        expires_at  TIMESTAMP NOT NULL,
                                        is_active   BOOLEAN   DEFAULT TRUE,
                                        created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS subscriptions (
                               id SERIAL PRIMARY KEY,
                               email VARCHAR(255),
                               license_key VARCHAR(50) UNIQUE,
                               plan_type VARCHAR(20), -- 'free', 'month', 'year'
                               status VARCHAR(20),    -- 'active', 'expired'
                               expires_at TIMESTAMP,  -- дата окончания
                               daily_mb_used FLOAT DEFAULT 0, -- сколько МБ сегодня потратил
                               daily_files_used INT DEFAULT 0, -- сколько файлов сегодня залил
                               last_action_date DATE DEFAULT CURRENT_DATE -- для сброса лимитов каждый день
);

-- Таблица для отслеживания бесплатных юзеров
CREATE TABLE  IF NOT EXISTS usage_stats (
                             id SERIAL PRIMARY KEY,
                             fingerprint TEXT UNIQUE NOT NULL,
                             ip_address TEXT,
                             count INTEGER DEFAULT 0,
                             updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Уникальный индекс, чтобы легко было делать UPSERT
CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_identity ON usage_stats (ip_address, fingerprint);

-- Добавим поля в подписки, если их нет
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS ip_address TEXT;
ALTER TABLE subscriptions ADD COLUMN IF NOT EXISTS fingerprint TEXT;