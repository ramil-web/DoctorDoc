package service

import (
    "doctordoc/internal/models"
    "github.com/xuri/excelize/v2"
    "log"
    "regexp"
    "strings"
    "time"
    "unicode"
)

func (s *fileService) processXLSX(meta *models.FileMetadata, save bool, userReq models.FixRequest) error {
    log.Printf("\n[API-LOG] >>> ЗАПУСК ОБРАБОТКИ (Save: %v)", save)

    f, err := excelize.OpenFile(meta.FilePath)
    if err != nil {
       log.Printf("[API-LOG] ❌ Ошибка открытия: %v", err)
       return err
    }
    defer f.Close()

    sheet := f.GetSheetName(0)
    rows, err := f.GetRows(sheet)
    if err != nil {
       return err
    }
    if len(rows) == 0 {
       return nil
    }

    headers := rows[0]
    meta.Errors = []models.FileError{}

    // Регулярки
    reEmail := regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
    reDigits := regexp.MustCompile(`\D`)
    reSimpleDate := regexp.MustCompile(`^(\d{2,4})[/-](\d{1,2})[/-](\d{2,4})$`)

    // Стиль "Текст" для сохранения пробелов и масок
    textStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 49})

    for i, row := range rows {
       if i == 0 { continue }

       // Проверка выбранных строк
       isRowSelected := true
       if len(userReq.SelectedRows) > 0 {
          isRowSelected = false
          for _, r := range userReq.SelectedRows {
             if r == i+1 { isRowSelected = true; break }
          }
       }
       if !isRowSelected { continue }

       for j, cell := range row {
          if j >= len(headers) { continue }
          colName := headers[j]
          if colName == "" { continue }

          // ТОТАЛЬНАЯ ОЧИСТКА: убираем неразрывные пробелы и прочий невидимый мусор
          cleanInput := strings.Map(func(r rune) rune {
             if unicode.IsSpace(r) { return ' ' }
             return r
          }, cell)
          trimmed := strings.TrimSpace(cleanInput)

          currentResult := cell
          appliedManual := false
          foundChange := false

          settings, hasSettings := userReq.Columns[colName]

          if !hasSettings {
             if cell != trimmed {
                currentResult = trimmed
                foundChange = true
             }
          } else {
             // 1. РУЧНОЙ ФОРМАТ (Приоритет юзера)
             if settings.Manual && settings.Format != "" {
                if strings.Contains(settings.Format, "X") {
                   currentResult = formatPhone(cell, settings.Format)
                } else if strings.Contains(settings.Format, "yyyy") || strings.Contains(settings.Format, "dd") {
                   currentResult = formatDate(cell, settings.Format)
                } else if strings.Contains(settings.Format, "234") || settings.Format == "int" {
                   currentResult = formatNumber(cell, settings.Format)
                }
                if currentResult != cell {
                   appliedManual = true
                   foundChange = true
                }
             }

             // 2. АВТО-ФОРМАТ
             if settings.Auto && !appliedManual && cell != "" {
                cleanDigits := reDigits.ReplaceAllString(trimmed, "")

                // Если это Email
                if strings.Contains(trimmed, "@") && reEmail.MatchString(strings.ReplaceAll(trimmed, " ", "")) {
                   res := strings.ToLower(strings.ReplaceAll(trimmed, " ", ""))
                   if res != cell {
                      currentResult = res
                      foundChange = true
                   }
                // Если это телефон (ищем именно цифры, игнорируя текст типа "оплата")
                } else if len(cleanDigits) >= 10 && len(cleanDigits) <= 12 {
                   res := "7" + cleanDigits[len(cleanDigits)-10:]
                   if res != cell {
                      currentResult = res
                      foundChange = true
                   }
                // Если это дата
                } else if !strings.Contains(trimmed, ":") && reSimpleDate.MatchString(trimmed) {
                   norm := regexp.MustCompile(`[/-]`).ReplaceAllString(trimmed, ".")
                   for _, l := range []string{"02.01.2006", "2006.01.02", "02.01.06"} {
                      if t, err := time.Parse(l, norm); err == nil {
                         res := t.Format("02.01.2006")
                         if res != cell {
                            currentResult = res
                            foundChange = true
                         }
                         break
                      }
                   }
                } else if cell != trimmed {
                   currentResult = trimmed
                   foundChange = true
                }
             }
          }

          // 3. ЗАПИСЬ
          if foundChange {
             meta.Errors = append(meta.Errors, models.FileError{
                Row: i + 1, Column: colName, OldValue: cell, NewValue: currentResult, Description: colName,
             })

             if save {
                cellAddr, _ := excelize.CoordinatesToCellName(j+1, i+1)
                // Сброс и жесткая запись строки со стилем ТЕКСТ
                f.SetCellValue(sheet, cellAddr, nil)
                f.SetCellStyle(sheet, cellAddr, cellAddr, textStyle)
                f.SetCellStr(sheet, cellAddr, currentResult)
                log.Printf("[WRITE-OK] %s: '%s' -> '%s'", cellAddr, cell, currentResult)
             }
          }
       }
    }

    if save {
       if err := f.SaveAs(meta.FilePath); err != nil {
          log.Printf("[API-LOG] ❌ Ошибка: %v", err)
          return err
       }
       log.Printf("[API-LOG] ✅ ФАЙЛ ПЕРЕЗАПИСАН: %s", meta.FilePath)
    }
    return nil
}

func formatPhone(val, mask string) string {
    digits := regexp.MustCompile(`\D`).ReplaceAllString(val, "")
    if len(digits) < 10 {
       return val
    }
    d := digits[len(digits)-10:]
    res := ""
    dIdx := 0
    for _, char := range mask {
       if char == 'X' && dIdx < len(d) {
          res += string(d[dIdx])
          dIdx++
       } else if char != 'X' {
          res += string(char)
       }
    }
    return res
}