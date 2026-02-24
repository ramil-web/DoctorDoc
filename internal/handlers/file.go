package handlers

import (
    "doctordoc/internal/models"
    "doctordoc/internal/service"
    "encoding/json"
    "fmt"
    "io"
    "net"
    "net/http"
    "os"
    "strings"
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

func getRealIP(r *http.Request) string {
    ip := r.Header.Get("X-Forwarded-For")
    if ip == "" {
       ip = r.RemoteAddr
    }
    if strings.Contains(ip, ",") {
       ip = strings.Split(ip, ",")[0]
    }
    if strings.Contains(ip, ":") {
       host, _, err := net.SplitHostPort(ip)
       if err == nil {
          ip = host
       }
    }
    return strings.TrimSpace(ip)
}

// GetDeviceID — теперь ID генерируется ТОЛЬКО на бэке, по IP
// hwData принимаем, но не используем (будет удалено позже)
func (h *FileHandler) GetDeviceID(w http.ResponseWriter, r *http.Request) {
    // читаем тело, чтобы не ломать контракт
    _ = json.NewDecoder(r.Body).Decode(&map[string]interface{}{})

    ip := getRealIP(r)
    deviceID := h.svc.GenerateHardwareHash(ip)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
       "device_id": deviceID,
    })
}

func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
    _ = r.ParseMultipartForm(10 << 20)

    file, header, err := r.FormFile("file")
    if err != nil {
       http.Error(w, "INVALID_FILE", http.StatusBadRequest)
       return
    }
    defer file.Close()

    // fingerprint с фронта игнорируется
    fp := r.Header.Get("X-Client-Fingerprint")

    ip := getRealIP(r)
    fileSizeMB := float64(header.Size) / (1024 * 1024)

    can, err := h.svc.CanUpload(r.Context(), fp, ip, fileSizeMB)
    if err != nil || !can {
       w.WriteHeader(http.StatusForbidden)
       errMsg := "LIMIT_EXCEEDED"
       if err != nil && err.Error() == "FILE_TOO_LARGE" {
          errMsg = "FILE_TOO_LARGE"
       }
       _ = json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
       return
    }

    content, err := io.ReadAll(file)
    if err != nil {
       http.Error(w, "READ_ERROR", http.StatusInternalServerError)
       return
    }

    id, err := h.svc.ProcessFile(r.Context(), header.Filename, content, fp)
    if err != nil {
       http.Error(w, err.Error(), http.StatusInternalServerError)
       return
    }

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (h *FileHandler) Download(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    ip := getRealIP(r)

    meta, err := h.svc.GetFileMeta(r.Context(), id)
    if err != nil {
       http.Error(w, "FILE_NOT_FOUND", http.StatusNotFound)
       return
    }

    file, err := os.Open(meta.FilePath)
    if err != nil {
       http.Error(w, "FILE_NOT_FOUND_ON_DISK", http.StatusNotFound)
       return
    }
    defer file.Close()

    info, _ := file.Stat()
    size := info.Size()

    w.Header().Set("Content-Disposition", "attachment; filename="+meta.OriginalName)
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Content-Length", fmt.Sprintf("%d", size))

    written, err := io.Copy(w, file)
    if err != nil {
       return
    }

    if written == size && !meta.IsDownloaded && meta.Status == "completed" {
       // usage учитывается по machineID (внутри сервиса)
       _ = h.svc.RecordDownload(r.Context(), id, ip)
       meta.IsDownloaded = true
       _ = h.svc.UpdateFileMeta(r.Context(), meta)
    }
}

func (h *FileHandler) Fix(w http.ResponseWriter, r *http.Request) {
    // В структуре FixRequest теперь Columns вместо CustomColumn/Format
    // Поэтому мы просто декодируем всё тело запроса сразу в модель
    var fixReq models.FixRequest

    if err := json.NewDecoder(r.Body).Decode(&fixReq); err != nil {
       http.Error(w, "INVALID_JSON", http.StatusBadRequest)
       return
    }

    ip := getRealIP(r)

    // Проверка лимитов (fingerprint игнорируем, работаем по IP)
    can, _ := h.svc.CanUpload(r.Context(), "", ip, 0)
    if !can {
       w.WriteHeader(http.StatusForbidden)
       _ = json.NewEncoder(w).Encode(map[string]string{"error": "LIMIT_EXCEEDED"})
       return
    }

    if err := h.svc.FixFile(r.Context(), fixReq); err != nil {
       http.Error(w, err.Error(), http.StatusInternalServerError)
       return
    }

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]string{"status": "completed"})
}

func (h *FileHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    status, errs, _ := h.svc.GetStatus(id)
    _ = json.NewEncoder(w).Encode(map[string]interface{}{
       "status": status,
       "errors": errs,
    })
}

func (h *FileHandler) Preview(w http.ResponseWriter, r *http.Request) {
    var req models.FixRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
       http.Error(w, "INVALID_JSON", http.StatusBadRequest)
       return
    }

    // Выполняем превью (без сохранения в файл)
    errors, err := h.svc.PreviewFile(r.Context(), req)
    if err != nil {
       http.Error(w, err.Error(), http.StatusInternalServerError)
       return
    }

    w.Header().Set("Content-Type", "application/json")
    _ = json.NewEncoder(w).Encode(map[string]interface{}{
       "status": "analyzed",
       "errors": errors,
    })
}

func (h *FileHandler) CheckLimit(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")

    // fingerprint с фронта игнорируем
    _ = r.Header.Get("X-Client-Fingerprint")

    ip := getRealIP(r)

    allowed, _ := h.svc.CanUpload(r.Context(), "", ip, 0)
    if !allowed {
       w.WriteHeader(http.StatusForbidden)
       _ = json.NewEncoder(w).Encode(map[string]string{"error": "LIMIT_EXCEEDED"})
       return
    }

    w.WriteHeader(http.StatusOK)
}

func logDownloadAction(fp, fileID string, count int, status string) {
    f, err := os.OpenFile("downloads_audit.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
       return
    }
    defer f.Close()

    line := fmt.Sprintf(
       "[%s] FP:%s | ID:%s | Count:%d | Status:%s\n",
       time.Now().Format("15:04:05"),
       fp,
       fileID,
       count,
       status,
    )
    _, _ = f.WriteString(line)
}