package handlers

import (
    "doctordoc/internal/models"
    "doctordoc/internal/service"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "time"

    "github.com/go-chi/chi/v5"
)

type FileHandler struct {
    svc    service.FileService
    subSvc service.SubscriptionService
}

func NewFileHandler(svc service.FileService, subSvc service.SubscriptionService) *FileHandler {
    return &FileHandler{svc: svc, subSvc: subSvc}
}

func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
    r.ParseMultipartForm(10 << 20)
    file, header, err := r.FormFile("file")
    if err != nil {
       http.Error(w, "INVALID_FILE", http.StatusBadRequest)
       return
    }
    defer file.Close()

    fp := r.FormValue("fingerprint")
    fileSizeMB := float64(header.Size) / (1024 * 1024)

    can, _ := h.svc.CanUpload(r.Context(), fp, fileSizeMB)
    if !can {
       w.WriteHeader(http.StatusForbidden)
       json.NewEncoder(w).Encode(map[string]string{"error": "LIMIT_EXCEEDED"})
       return
    }

    content, _ := io.ReadAll(file)
    id, err := h.svc.ProcessFile(r.Context(), header.Filename, content, fp)
    if err != nil {
       http.Error(w, err.Error(), http.StatusInternalServerError)
       return
    }

    json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (h *FileHandler) Download(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    queryFP := r.URL.Query().Get("fp")

    meta, err := h.svc.GetFileMeta(r.Context(), id)
    if err != nil {
        http.Error(w, "FILE_NOT_FOUND", http.StatusNotFound)
        return
    }

    // Привязываем FP если его нет
    if meta.Fingerprint == "" && queryFP != "" {
        meta.Fingerprint = queryFP
        h.svc.UpdateFileMeta(r.Context(), meta)
    }

    // Открываем файл
    file, err := os.Open(meta.FilePath)
    if err != nil {
        http.Error(w, "FILE_NOT_FOUND_ON_DISK", http.StatusNotFound)
        return
    }
    defer file.Close()

    // Получаем информацию о файле для размера
    fileInfo, _ := file.Stat()
    fileSize := fileInfo.Size()

    // Устанавливаем заголовки
    w.Header().Set("Content-Disposition", "attachment; filename="+meta.OriginalName)
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))

    // 🚀 СТРИМИНГ ФАЙЛА С ПРОВЕРКОЙ
    // io.Copy будет ждать, пока все байты не уйдут клиенту
    written, err := io.Copy(w, file)

    if err != nil {
        fmt.Printf("❌ [DOWNLOAD] Обрыв соединения для %s: %v\n", id, err)
        return
    }

    // ПРОВЕРКА: Списываем лимит только если файл передан ПОЛНОСТЬЮ
    if written == fileSize {
        fmt.Printf("✅ [DOWNLOAD] Файл %s успешно доставлен (%d байт)\n", id, written)

        if !meta.IsDownloaded && meta.Fingerprint != "" && meta.Status == "completed" {
            newCount, err := h.subSvc.IncrementUsageWithCount(r.Context(), meta.Fingerprint)
            if err == nil {
                meta.IsDownloaded = true
                h.svc.UpdateFileMeta(r.Context(), meta)
                logDownloadAction(meta.Fingerprint, id, newCount, "SUCCESS_FULL_DELIVERY")
            }
        }
    } else {
        fmt.Printf("⚠️ [DOWNLOAD] Файл передан частично: %d из %d байт. Лимит не списан.\n", written, fileSize)
    }
}

func (h *FileHandler) Fix(w http.ResponseWriter, r *http.Request) {
    var req struct {
       ID            string `json:"id"`
       LicenseNumber string `json:"license_number"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    fixReq := models.FixRequest{ID: req.ID, LicenseNumber: req.LicenseNumber}
    if err := h.svc.FixFile(r.Context(), fixReq); err != nil {
       http.Error(w, err.Error(), http.StatusInternalServerError)
       return
    }

    json.NewEncoder(w).Encode(map[string]string{"status": "completed"})
}

func (h *FileHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    status, errs, _ := h.svc.GetStatus(id)
    json.NewEncoder(w).Encode(map[string]interface{}{"status": status, "errors": errs})
}

func logDownloadAction(fp, fileID string, count int, status string) {
    f, err := os.OpenFile("downloads_audit.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil { return }
    defer f.Close()
    line := fmt.Sprintf("[%s] FP: %s | ID: %s | Count: %d | Status: %s\n", time.Now().Format("15:04:05"), fp, fileID, count, status)
    f.WriteString(line)
}