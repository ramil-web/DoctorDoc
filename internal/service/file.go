package service

import (
    "context"
    "encoding/csv"
    "fmt"
    "io"
    "log"
    "os"
    "path/filepath"
    "regexp"
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
    CanUpload(ctx context.Context, fingerprint string, fileSizeMB float64) (bool, error)
    CheckOnlyLimit(ctx context.Context, fingerprint string) (bool, error)
}

type fileService struct {
    repo repository.Repository
    rdb  *redis.Client
}

func NewFileService(repo repository.Repository, rdb *redis.Client) FileService {
    return &fileService{repo: repo, rdb: rdb}
}

func (s *fileService) CheckOnlyLimit(ctx context.Context, fingerprint string) (bool, error) {
    if fingerprint == "" {
       return true, nil
    }
    count, err := s.repo.GetUsageCount(ctx, fingerprint)
    if err != nil {
       return true, nil
    }
    return count < 3, nil
}

func (s *fileService) CanUpload(ctx context.Context, fingerprint string, fileSizeMB float64) (bool, error) {
    if fingerprint == "" {
       return true, nil
    }

    hasSub, _ := s.repo.CheckActiveSubscription("", fingerprint)
    if hasSub {
       return true, nil
    }

    if fileSizeMB > 5.0 {
       return false, fmt.Errorf("LIMIT_SIZE_EXCEEDED")
    }

    count, err := s.repo.GetUsageCount(ctx, fingerprint)
    if err != nil {
       return true, nil
    }

    return count < 3, nil
}

// ИСПРАВЛЕНО: реализация теперь точно совпадает с интерфейсом
func (s *fileService) ProcessFile(ctx context.Context, name string, content []byte, fp string) (string, error) {
    id := uuid.New().String()
    path := filepath.Join("./uploads", id+filepath.Ext(name))
    _ = os.MkdirAll("./uploads", 0755)

    if err := os.WriteFile(path, content, 0644); err != nil {
        return "", err
    }

    meta := models.FileMetadata{
        ID:           id,
        OriginalName: name,
        FilePath:     path,
        Status:       "uploaded",
        CreatedAt:    time.Now(),
        Fingerprint:  fp, // Сохраняем отпечаток в БД
    }

    if err := s.repo.SaveFileMeta(ctx, meta); err != nil {
        return "", err
    }

    s.rdb.LPush(ctx, "file_processing_queue", id)
    return id, nil
}

func (s *fileService) StartWorker(ctx context.Context) {
    log.Println("👷 Воркер анализа файлов запущен...")
    for i := 0; i < 5; i++ {
       go func() {
          for {
             res, err := s.rdb.BLPop(ctx, 0, "file_processing_queue").Result()
             if err != nil {
                continue
             }
             fileID := res[1]

             meta, err := s.repo.GetFileMeta(ctx, fileID)
             if err != nil {
                continue
             }

             meta.Status = "processing"
             s.repo.SaveFileMeta(ctx, *meta)

             if err := s.processCSV(meta, false); err != nil {
                log.Printf("❌ Ошибка обработки %s: %v", fileID, err)
                meta.Status = "error"
             } else {
                meta.Status = "analyzed"
             }

             s.repo.SaveFileMeta(ctx, *meta)
             log.Printf("✅ Анализ завершен: %s (Ошибок: %d)", fileID, len(meta.Errors))
          }
       }()
    }
}

func (s *fileService) processCSV(meta *models.FileMetadata, save bool) error {
    f, err := os.Open(meta.FilePath)
    if err != nil { return err }
    defer f.Close()

    reader := csv.NewReader(f)
    reader.LazyQuotes = true
    reader.FieldsPerRecord = -1

    var writer *csv.Writer
    var tempFile *os.File
    if save {
       tempFile, _ = os.Create(meta.FilePath + ".tmp")
       defer tempFile.Close()
       writer = csv.NewWriter(tempFile)
    }

    meta.Errors = []models.FileError{}
    line := 0
    for {
       record, err := reader.Read()
       if err == io.EOF { break }
       if err != nil { continue }
       line++

       for j := range record {
          newVal, errs := s.cleanValue(record[j], line, j+1)
          if save {
             record[j] = newVal
          } else {
             if len(meta.Errors) < 2000 {
                meta.Errors = append(meta.Errors, errs...)
             }
          }
       }
       if save { writer.Write(record) }
    }
    meta.RowsCount = line
    if save {
       writer.Flush()
       return os.Rename(meta.FilePath+".tmp", meta.FilePath)
    }
    return nil
}

func (s *fileService) cleanValue(val string, row, col int) (string, []models.FileError) {
    var errs []models.FileError
    re := regexp.MustCompile(`(\+7|7|8)?[\s\-]?\(?[9][0-9]{2}\)?[\s\-]?[0-9]{3}[\s\-]?[0-9]{2}[\s\-]?[0-9]{2}`)

    found := re.FindString(val)
    if found != "" {
       digits := regexp.MustCompile(`\D`).ReplaceAllString(found, "")
       if len(digits) >= 10 {
          norm := "7" + digits[len(digits)-10:]
          if norm != found && norm != digits {
             errs = append(errs, models.FileError{
                Row:         row,
                Column:      fmt.Sprintf("Колонка %d", col),
                OldValue:    found,
                NewValue:    norm,
                Description: "Неверный формат номера телефона",
             })
             val = strings.ReplaceAll(val, found, norm)
          }
       }
    }
    return val, errs
}

func (s *fileService) GetStatus(id string) (string, []models.FileError, error) {
    meta, err := s.repo.GetFileMeta(context.Background(), id)
    if err != nil { return "", nil, err }
    return meta.Status, meta.Errors, nil
}

func (s *fileService) FixFile(ctx context.Context, req models.FixRequest) error {
    meta, err := s.repo.GetFileMeta(ctx, req.ID)
    if err != nil { return err }
    if err := s.processCSV(meta, true); err != nil { return err }
    meta.Status = "completed"
    return s.repo.SaveFileMeta(ctx, *meta)
}

func (s *fileService) GetFileMeta(ctx context.Context, id string) (models.FileMetadata, error) {
    m, err := s.repo.GetFileMeta(ctx, id)
    if err != nil { return models.FileMetadata{}, err }
    return *m, nil
}

func (s *fileService) GetFilePath(ctx context.Context, id string) (string, error) {
    m, err := s.repo.GetFileMeta(ctx, id)
    if err != nil { return "", err }
    return m.FilePath, nil
}

func (s *fileService) UpdateFileMeta(ctx context.Context, meta models.FileMetadata) error {
    return s.repo.SaveFileMeta(ctx, meta)
}