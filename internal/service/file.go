package service

import (
    "context"
    "crypto/sha256"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"
    "time"

    "doctordoc/internal/models"
    "doctordoc/internal/repository"

    "github.com/go-redis/redis/v8"
    "github.com/google/uuid"
)

type FileService interface {
    StartWorker(ctx context.Context)
    UpdateFileMeta(ctx context.Context, meta models.FileMetadata) error
    ProcessFile(ctx context.Context, name string, content []byte, fp string) (string, error)
    GetStatus(id string) (string, []models.FileError, error)
    FixFile(ctx context.Context, req models.FixRequest) error
    GetFileMeta(ctx context.Context, id string) (models.FileMetadata, error)
    GetFilePath(ctx context.Context, id string) (string, error)
    CanUpload(ctx context.Context, fingerprint string, ip string, fileSizeMB float64) (bool, error)
    CheckOnlyLimit(ctx context.Context, fingerprint string, ip string) (bool, error)
    RecordDownload(ctx context.Context, fileID string, ip string) error
    GenerateHardwareHash(ip string) string
}

type fileService struct {
    repo repository.Repository
    rdb  *redis.Client
}

func NewFileService(repo repository.Repository, rdb *redis.Client) FileService {
    return &fileService{repo: repo, rdb: rdb}
}

func (s *fileService) GenerateHardwareHash(ip string) string {
    const salt = "HARD_LOCK_2026_STABLE"
    raw := fmt.Sprintf("%s|%s", ip, salt)
    hash := sha256.Sum256([]byte(raw))
    return fmt.Sprintf("hw_%x", hash)
}

func (s *fileService) StartWorker(ctx context.Context) {
    log.Println("👷 Worker started")
    for i := 0; i < 5; i++ {
       go func() {
          for {
             res, err := s.rdb.BLPop(ctx, 0, "file_processing_queue").Result()
             if err != nil {
                continue
             }

             fileID := res[1]
             meta, _ := s.repo.GetFileMeta(ctx, fileID)
             if meta == nil {
                continue
             }

             meta.Status = "processing"
             s.repo.SaveFileMeta(ctx, *meta)

             ext := strings.ToLower(filepath.Ext(meta.OriginalName))
             switch ext {
             case ".xlsx":
                s.processXLSX(meta, false)
             case ".txt":
                s.processTXT(meta, false)
             default:
                s.processCSV(meta, false)
             }

             meta.Status = "analyzed"
             s.repo.SaveFileMeta(ctx, *meta)
          }
       }()
    }
}

func (s *fileService) ProcessFile(ctx context.Context, name string, content []byte, _ string) (string, error) {
    id := uuid.New().String()
    path := filepath.Join("./uploads", id+filepath.Ext(name))

    _ = os.MkdirAll("./uploads", 0755)
    _ = os.WriteFile(path, content, 0644)

    meta := models.FileMetadata{
       ID:           id,
       OriginalName: name,
       FilePath:     path,
       Status:       "uploaded",
       CreatedAt:    time.Now(),
    }

    s.repo.SaveFileMeta(ctx, meta)
    s.rdb.LPush(ctx, "file_processing_queue", id)

    return id, nil
}

func (s *fileService) FixFile(ctx context.Context, req models.FixRequest) error {
    meta, _ := s.repo.GetFileMeta(ctx, req.ID)

    ext := strings.ToLower(filepath.Ext(meta.OriginalName))
    switch ext {
    case ".xlsx":
       s.processXLSX(meta, true)
    case ".txt":
       s.processTXT(meta, true)
    default:
       s.processCSV(meta, true)
    }

    meta.Status = "completed"
    return s.repo.SaveFileMeta(ctx, *meta)
}

func (s *fileService) GetStatus(id string) (string, []models.FileError, error) {
    // Используем контекст и правильное имя метода
    meta, err := s.repo.GetFileMeta(context.Background(), id)
    if err != nil {
       return "error", nil, err
    }
    return meta.Status, meta.Errors, nil
}

func (s *fileService) GetFileMeta(ctx context.Context, id string) (models.FileMetadata, error) {
    m, err := s.repo.GetFileMeta(ctx, id)
    if err != nil || m == nil {
       return models.FileMetadata{}, fmt.Errorf("metadata not found")
    }
    return *m, nil
}

func (s *fileService) UpdateFileMeta(ctx context.Context, meta models.FileMetadata) error {
    return s.repo.SaveFileMeta(ctx, meta)
}

func (s *fileService) GetFilePath(ctx context.Context, id string) (string, error) {
    m, err := s.repo.GetFileMeta(ctx, id)
    if err != nil || m == nil {
       return "", fmt.Errorf("file not found")
    }
    return m.FilePath, nil
}

func (s *fileService) RecordDownload(ctx context.Context, fileID string, ip string) error {
    if ip == "" { return nil }
    meta, err := s.repo.GetFileMeta(ctx, fileID)
    if err != nil || meta == nil { return fmt.Errorf("file not found") }

    machineID := s.GenerateHardwareHash(ip)
    _, err = s.repo.IncrementUsage(ctx, machineID, ip)
    return err
}

func (s *fileService) createError(row int, col, old, new, desc string) models.FileError {
    return models.FileError{
       Row:         row,
       Column:      strings.ToUpper(col),
       OldValue:    old,
       NewValue:    new,
       Description: desc,
    }
}