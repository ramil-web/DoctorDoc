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
    PreviewFile(ctx context.Context, req models.FixRequest) ([]models.FileError, error)
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
    return fmt.Sprintf("hw_%x", sha256.Sum256([]byte(fmt.Sprintf("%s|HARD_LOCK_2026_STABLE", ip))))
}

func (s *fileService) StartWorker(ctx context.Context) {
    log.Println("👷 Worker started")
    for i := 0; i < 5; i++ {
       go func() {
          for {
             res, err := s.rdb.BLPop(ctx, 0, "file_processing_queue").Result()
             if err != nil { continue }
             fileID := res[1]
             meta, _ := s.repo.GetFileMeta(ctx, fileID)
             if meta == nil { continue }
             meta.Status = "processing"
             s.repo.SaveFileMeta(ctx, *meta)

             ext := strings.ToLower(filepath.Ext(meta.OriginalName))

             // Инициализируем мапу, чтобы процессоры (xlsx/csv) не скипали анализ
             analyzeReq := models.FixRequest{
                 ID:      fileID,
                 Columns: make(map[string]models.ColumnSettings),
             }

             switch ext {
             case ".xlsx": s.processXLSX(meta, false, analyzeReq)
             case ".txt": s.processTXT(meta, false, analyzeReq)
             default: s.processCSV(meta, false, analyzeReq)
             }

             meta.Status = "analyzed"
             s.repo.SaveFileMeta(ctx, *meta)
             log.Printf("✅ Файл %s проанализирован. Найдено ошибок: %d", fileID, len(meta.Errors))
          }
       }()
    }
}

func (s *fileService) ProcessFile(ctx context.Context, name string, content []byte, _ string) (string, error) {
    id := uuid.New().String(); path := filepath.Join("./uploads", id+filepath.Ext(name))
    _ = os.MkdirAll("./uploads", 0755); _ = os.WriteFile(path, content, 0644)
    meta := models.FileMetadata{ID: id, OriginalName: name, FilePath: path, Status: "uploaded", CreatedAt: time.Now()}
    s.repo.SaveFileMeta(ctx, meta); s.rdb.LPush(ctx, "file_processing_queue", id)
    return id, nil
}

func (s *fileService) PreviewFile(ctx context.Context, req models.FixRequest) ([]models.FileError, error) {
    log.Printf("[API/Preview] 🔎 Req for ID: %s, Columns: %v", req.ID, len(req.Columns))
    meta, err := s.repo.GetFileMeta(ctx, req.ID)
    if err != nil {
        log.Printf("[API/Preview] ❌ Meta not found: %v", err)
        return nil, err
    }

    ext := strings.ToLower(filepath.Ext(meta.OriginalName))
    meta.Errors = []models.FileError{}

    switch ext {
    case ".xlsx": s.processXLSX(meta, false, req)
    case ".txt":  s.processTXT(meta, false, req)
    default:      s.processCSV(meta, false, req)
    }

    log.Printf("[API/Preview] ✅ Done. Returning %d errors", len(meta.Errors))
    return meta.Errors, nil
}

func (s *fileService) FixFile(ctx context.Context, req models.FixRequest) error {
    log.Printf("[API/Fix] 🛠️ Req for ID: %s", req.ID)
    meta, _ := s.repo.GetFileMeta(ctx, req.ID)
    ext := strings.ToLower(filepath.Ext(meta.OriginalName))
    switch ext {
    case ".xlsx": s.processXLSX(meta, true, req)
    case ".txt": s.processTXT(meta, true, req)
    default: s.processCSV(meta, true, req)
    }
    meta.Status = "completed"
    log.Printf("[API/Fix] ✅ Completed for ID: %s", req.ID)
    return s.repo.SaveFileMeta(ctx, *meta)
}

func (s *fileService) GetStatus(id string) (string, []models.FileError, error) {
    meta, err := s.repo.GetFileMeta(context.Background(), id)
    if err != nil { return "error", nil, err }
    return meta.Status, meta.Errors, nil
}

func (s *fileService) GetFileMeta(ctx context.Context, id string) (models.FileMetadata, error) {
    m, err := s.repo.GetFileMeta(ctx, id)
    if err != nil || m == nil { return models.FileMetadata{}, fmt.Errorf("not found") }
    return *m, nil
}

func (s *fileService) UpdateFileMeta(ctx context.Context, meta models.FileMetadata) error { return s.repo.SaveFileMeta(ctx, meta) }

func (s *fileService) GetFilePath(ctx context.Context, id string) (string, error) {
    m, _ := s.repo.GetFileMeta(ctx, id); if m == nil { return "", nil }; return m.FilePath, nil
}

func (s *fileService) RecordDownload(ctx context.Context, id, ip string) error {
    machineID := s.GenerateHardwareHash(ip); _, err := s.repo.IncrementUsage(ctx, machineID, ip); return err
}