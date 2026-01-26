package service

import (
    "doctordoc/internal/models"
    "encoding/csv"
    "log"
    "os"
    "regexp"
    "strings"
    "sync"
    "time"
)

func (s *fileService) processCSV(meta *models.FileMetadata, save bool, userReq models.FixRequest) error {
    log.Printf("\n[API-LOG] >>> ЗАПУСК ОБРАБОТКИ CSV (Save: %v)", save)

    content, err := os.ReadFile(meta.FilePath)
    if err != nil {
       return err
    }

    raw := string(content)
    sep := ';'
    if strings.Count(raw, ",") > strings.Count(raw, ";") {
       sep = ','
    }

    reader := csv.NewReader(strings.NewReader(raw))
    reader.Comma = sep
    reader.LazyQuotes = true
    reader.TrimLeadingSpace = false // Ставим false, чтобы вручную контролировать пробелы

    records, err := reader.ReadAll()
    if err != nil || len(records) == 0 {
       return err
    }

    headers := make([]string, len(records[0]))
    for i, h := range records[0] {
       headers[i] = strings.TrimSpace(h)
    }

    meta.Errors = []models.FileError{}

    reEmail := regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
    reDigits := regexp.MustCompile(`\D`)
    reDateLike := regexp.MustCompile(`(\d{1,4})[./\s,\\\-_](\d{1,2})[./\s,\\\-_](\d{2,4})`)
    reSpaces := regexp.MustCompile(`\s+`)

    selectedMap := make(map[int]bool)
    for _, r := range userReq.SelectedRows {
       selectedMap[r] = true
    }

    var wg sync.WaitGroup
    var mu sync.Mutex
    semaphore := make(chan struct{}, 100)

    for i := 1; i < len(records); i++ {
       if len(selectedMap) > 0 && !selectedMap[i+1] {
          continue
       }

       wg.Add(1)
       semaphore <- struct{}{}

       go func(idx int) {
          defer wg.Done()
          defer func() { <-semaphore }()

          row := records[idx]

          for j, cell := range row {
             if j >= len(headers) { continue }
             colName := headers[j]

             currentResult := cell
             foundChange := false
             settings, hasSettings := userReq.Columns[colName]

             trimmed := strings.TrimSpace(cell)
             normalizedSpaces := reSpaces.ReplaceAllString(trimmed, " ")

             // 1. ЛОГИКА ОБРАБОТКИ
             if hasSettings && settings.Manual && settings.Format != "" {
                var resManual string
                if strings.Contains(settings.Format, "X") || strings.Contains(colName, "телефон") {
                   resManual = formatPhone(cell, settings.Format)
                } else if strings.Contains(settings.Format, "yyyy") || strings.Contains(settings.Format, "dd") {
                   resManual = formatDate(cell, settings.Format)
                } else if strings.Contains(settings.Format, "234") || settings.Format == "int" {
                   resManual = formatNumber(cell, settings.Format)
                }
                if resManual != "" && resManual != cell {
                   currentResult = resManual
                   foundChange = true
                }
             } else if !hasSettings || settings.Auto {
                if normalizedSpaces != "" {
                   noSpaces := strings.ReplaceAll(normalizedSpaces, " ", "")

                   // Дата
                   if match := reDateLike.FindStringSubmatch(normalizedSpaces); match != nil && !strings.Contains(normalizedSpaces, "@") {
                      d, m, y := match[1], match[2], match[3]
                      if len(d) == 1 { d = "0" + d }
                      if len(m) == 1 { m = "0" + m }
                      if len(y) == 2 { y = "20" + y }
                      normDate := d + "." + m + "." + y
                      if t, err := time.Parse("02.01.2006", normDate); err == nil {
                         res := t.Format("02.01.2006")
                         if res != cell { currentResult = res; foundChange = true }
                      }
                   }

                   // Почта
                   if !foundChange && strings.Contains(noSpaces, "@") && reEmail.MatchString(noSpaces) {
                      res := strings.ToLower(noSpaces)
                      if res != cell { currentResult = res; foundChange = true }
                   }

                   // Телефон
                   if !foundChange {
                      cd := reDigits.ReplaceAllString(normalizedSpaces, "")
                      if len(cd) >= 10 && len(cd) <= 12 {
                         mask := "7XXXXXXXXXX"
                         if hasSettings && settings.Format != "" { mask = settings.Format }
                         res := formatPhone(normalizedSpaces, mask)
                         if res != cell { currentResult = res; foundChange = true }
                      }
                   }

                   // Цена
                   if !foundChange {
                      if regexp.MustCompile(`\d`).MatchString(normalizedSpaces) && (strings.Contains(normalizedSpaces, "руб") || strings.Contains(normalizedSpaces, ",")) {
                         res := formatNumber(normalizedSpaces, "")
                         if res != "" && res != cell { currentResult = res; foundChange = true }
                      }
                   }

                   if !foundChange && normalizedSpaces != cell {
                      currentResult = normalizedSpaces
                      foundChange = true
                   }
                } else if cell != "" {
                   currentResult = ""
                   foundChange = true
                }
             }

             // 2. ЗАПИСЬ И ЛОГИРОВАНИЕ
             if foundChange {
                // В логах используем елочки для наглядности пробелов
                log.Printf("[CSV/FIX] Row: %d | Col: %s | «%s» -> «%s»", idx+1, colName, cell, currentResult)

                mu.Lock()
                meta.Errors = append(meta.Errors, models.FileError{
                   Row:         idx + 1,
                   Column:      colName,
                   OldValue:    cell,
                   NewValue:    currentResult,
                   Description: colName,
                })
                if save {
                   records[idx][j] = currentResult
                }
                mu.Unlock()
             }
          }
       }(i)
    }

    wg.Wait()

    if save {
       f, err := os.Create(meta.FilePath)
       if err != nil { return err }
       defer f.Close()
       w := csv.NewWriter(f)
       w.Comma = sep
       w.WriteAll(records)
       log.Printf("[API-LOG] ✅ ФАЙЛ ПЕРЕЗАПИСАН")
    }
    return nil
}