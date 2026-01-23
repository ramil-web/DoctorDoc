package service

import (
    "doctordoc/internal/models"
    "os"
    "strings"
    "sync"
)

func (s *fileService) processTXT(meta *models.FileMetadata, save bool, userReq models.FixRequest) error {
    content, err := os.ReadFile(meta.FilePath)
    if err != nil { return err }
    lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
    var wg sync.WaitGroup
    var mu sync.Mutex

    // Настройки для TXT берем по ключу "ТЕКСТ"
    settings, hasSettings := userReq.Columns["ТЕКСТ"]

    for i, line := range lines {
       if line == "" { continue }

       // Проверка выбранных строк
       isRowSelected := true
       if len(userReq.SelectedRows) > 0 {
          isRowSelected = false
          for _, r := range userReq.SelectedRows {
             if r == i+1 { isRowSelected = true; break }
          }
       }
       if !isRowSelected { continue }

       wg.Add(1)
       go func(idx int, rawLine string) {
          defer wg.Done()
          if !hasSettings { return }

          res := strings.TrimSpace(rawLine); changed := false

          // РУЧНОЙ ФОРМАТ
          if settings.Manual && settings.Format != "" {
             if strings.Contains(settings.Format, "X") {
                res = formatPhone(rawLine, settings.Format); changed = true
             } else if strings.Contains(settings.Format, "yyyy") || strings.Contains(settings.Format, "dd") {
                res = formatDate(rawLine, settings.Format); changed = true
             }
          }

          // АВТО-ПРАВКА (если ручной не сработал)
          if !changed && settings.Auto {
             if rawLine != strings.TrimSpace(rawLine) {
                res = strings.TrimSpace(rawLine); changed = true
             }
          }

          if changed || rawLine != strings.TrimSpace(rawLine) {
             mu.Lock()
             meta.Errors = append(meta.Errors, models.FileError{Row: idx + 1, Column: "ТЕКСТ", OldValue: rawLine, NewValue: res, Description: "ТЕКСТ"})
             if save { lines[idx] = res }
             mu.Unlock()
          }
       }(i, line)
    }
    wg.Wait()
    if save { os.WriteFile(meta.FilePath, []byte(strings.Join(lines, "\n")), 0644) }
    return nil
}