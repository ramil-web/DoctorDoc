package models

import "time"

// FileError описывает конкретное исправление в ячейке
type FileError struct {
	Row         int    `json:"row"`
	Column      string `json:"column"`
	OldValue    string `json:"old_value"`
	NewValue    string `json:"new_value"`
	Description string `json:"description"`
}

// ProcessingStats хранит итоговые цифры для дашборда
type ProcessingStats struct {
	PhonesFixed int `json:"phones_fixed"`
	DatesFixed  int `json:"dates_fixed"`
	EmailsFixed int `json:"emails_fixed"`
	TotalErrors int `json:"total_errors"`
}

type FileMetadata struct {
	ID           string          `json:"id"`
	OriginalName string          `json:"original_name"`
	FilePath     string          `json:"file_path"`
	RowsCount    int             `json:"rows_count"`
	Status       string          `json:"status"`
	CreatedAt    time.Time       `json:"created_at"`
	Fingerprint  string          `json:"fingerprint"`
	IsDownloaded bool            `json:"is_downloaded"`
	Stats        ProcessingStats `json:"stats"`
	Errors       []FileError     `json:"errors"`
}

type License struct {
	KeyHash     string    `json:"key_hash"`
	Tariff      string    `json:"tariff"`
	Fingerprint string    `json:"fingerprint"`
	ExpiresAt   time.Time `json:"expires_at"`
	IsActive    bool      `json:"is_active"`
}

type FixRequest struct {
    ID            string   `json:"id"`
    LicenseNumber string   `json:"license_number"`
    CustomColumn  string   `json:"custom_column"`
    CustomFormat  string   `json:"custom_format"`
    SelectedRows  []int    `json:"selected_rows"`
}

type Subscription struct {
    Email           string    `json:"email"`
    PlanType        string    `json:"plan_type"`
    Status          string    `json:"status"`
    ExpiresAt       time.Time `json:"expires_at"`
    DailyMbUsed     float64   `json:"daily_mb_used"`
    DailyFilesUsed  int       `json:"daily_files_used"`
    LastActivityAt  time.Time `json:"last_activity_at"`
}


type SupportRequest struct {
    Name     string `json:"name"`
    Email    string `json:"email"`
    Telegram string `json:"telegram"`
    Text     string `json:"text"`
}