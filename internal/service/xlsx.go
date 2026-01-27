package service

import (
    "doctordoc/internal/models"
    "fmt"
    "github.com/xuri/excelize/v2"
    "log"
    "regexp"
    "strings"
    "unicode"
)

func (s *fileService) processXLSX(meta *models.FileMetadata, save bool, userReq models.FixRequest) error {
    log.Printf("\n[API-LOG] >>> СТАРТ ОБРАБОТКИ. Файл: %s, Сохранение: %v", meta.FilePath, save)
    log.Printf("[API-LOG] 📥 ПОЛУЧЕНО ОТ ФРОНТЕНДА (userReq): %+v", userReq)

    f, err := excelize.OpenFile(meta.FilePath)
    if err != nil {
       log.Printf("[API-LOG] ❌ Ошибка открытия файла: %v", err)
       return err
    }
    defer f.Close()

    sheet := f.GetSheetName(0)
    rows, err := f.GetRows(sheet)
    if err != nil {
       return err
    }
    if len(rows) == 0 {
       log.Printf("[API-LOG] ⚠️ Файл пуст")
       return nil
    }

    headers := rows[0]
    log.Printf("[API-LOG] 📋 Заголовки в файле: %v", headers)

    meta.Errors = []models.FileError{}
    for _, h := range headers {
        meta.Errors = append(meta.Errors, models.FileError{
            Row:         0,
            Column:      h,
            Description: h,
        })
    }

    if len(userReq.Columns) == 0 {
       log.Printf("[API-LOG] 📥 Настройки пусты. Авто-инициализация...")
       userReq.Columns = make(map[string]models.ColumnSettings)
       for _, h := range headers {
          userReq.Columns[h] = models.ColumnSettings{Auto: true}
       }
    }

    colSettingsByIndex := make(map[int]models.ColumnSettings)
    for j, hName := range headers {
       hClean := strings.ToLower(strings.TrimSpace(hName))
       excelLetter, _ := excelize.ColumnNumberToName(j + 1)
       excelLetterLower := strings.ToLower(excelLetter)

       for reqColKey, settings := range userReq.Columns {
          reqClean := strings.ToLower(strings.TrimSpace(reqColKey))
          if reqClean == hClean || reqClean == excelLetterLower || reqClean == fmt.Sprintf("%d", j) {
             colSettingsByIndex[j] = settings
             log.Printf("[API-LOG] ✅ СВЯЗКА УСТАНОВЛЕНА: Колонка %d (%s) <---> Ключ запроса '%s'. Настройки: Manual=%v, Auto=%v, Format='%s'",
                j, excelLetter, reqColKey, settings.Manual, settings.Auto, settings.Format)
          }
       }
    }

    reEmail := regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
    reDigits := regexp.MustCompile(`\d`)
    reOnlyDigits := regexp.MustCompile(`\D`)
    reHasLetters := regexp.MustCompile(`(?i)[a-zа-яё]`)

    rePotentialPrice := regexp.MustCompile(`(?i)(\d+[\s.,]?\d*\s?(руб|р\.|\bрубл[яьей]\b|₽|\$|€|¥))`)
    reSimpleDate := regexp.MustCompile(`^(\d{1,4})[.,/\- ](\d{1,2})[.,/\- ](\d{1,4})`)

    textStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 49})

    for i, row := range rows {
       if i == 0 {
          continue
       }

       for j, cell := range row {
          if j >= len(headers) {
             continue
          }

          rawColName := headers[j]
          excelAddr, _ := excelize.CoordinatesToCellName(j+1, i+1)
          settings, hasSettings := colSettingsByIndex[j]

          cleanInput := strings.Map(func(r rune) rune {
             if unicode.IsSpace(r) {
                return ' '
             }
             return r
          }, cell)
          trimmed := strings.TrimSpace(cleanInput)

          currentResult := cell
          foundChange := false
          wasManual := false

          if !hasSettings {
             if cell != trimmed {
                currentResult = trimmed
                foundChange = true
             }
          } else {
             // 1. РУЧНОЕ ФОРМАТИРОВАНИЕ
             if settings.Manual && settings.Format != "" {
                fLower := strings.ToLower(settings.Format)
                var manualRes string

                isDate := strings.ContainsAny(fLower, "ymd")
                isCurrency := strings.ContainsAny(fLower, "₽$€¥") ||
                   regexp.MustCompile(`(?i)(руб|р\.|\bрубл[яьей]\b)`).MatchString(fLower)

                isNumericMask := regexp.MustCompile(`^[0-9+\-() ]+$`).MatchString(settings.Format)

                if (strings.Contains(fLower, "x") || isNumericMask) && !isDate {
                   if isCurrency || strings.ContainsAny(fLower, ".,") || strings.Contains(fLower, "x x") {
                      manualRes = formatNumber(cell, settings.Format)
                   } else {
                      if len(reOnlyDigits.ReplaceAllString(fLower, "")) == 0 && len(fLower) <= 6 && !strings.ContainsAny(fLower, "+()-") {
                         manualRes = reOnlyDigits.ReplaceAllString(trimmed, "")
                      } else {
                         manualRes = formatPhone(cell, settings.Format)
                      }
                   }
                } else if isDate {
                   manualRes = formatDate(cell, settings.Format)
                } else if strings.Contains(fLower, "int") || strings.Contains(fLower, "234") {
                   manualRes = formatNumber(cell, settings.Format)
                }

                if manualRes != "" && manualRes != cell {
                   currentResult = manualRes
                   foundChange = true
                   wasManual = true
                }
             }

             // 2. АВТО ФОРМАТИРОВАНИЕ
             if settings.Auto && !foundChange && cell != "" {
                cleanDigits := reOnlyDigits.ReplaceAllString(trimmed, "")

                if strings.Contains(trimmed, "@") && reEmail.MatchString(strings.ReplaceAll(trimmed, " ", "")) {
                   res := strings.ToLower(strings.ReplaceAll(trimmed, " ", ""))
                   if res != cell {
                      currentResult = res
                      foundChange = true
                   }
                }

                // PHONE
                if !foundChange && len(cleanDigits) >= 10 && len(cleanDigits) <= 12 && !reHasLetters.MatchString(trimmed) {
                   mask := "7XXXXXXXXXX"
                   if settings.Format != "" {
                      mask = settings.Format
                   }
                   res := formatPhone(trimmed, mask)
                   if res != cell {
                      currentResult = res
                      foundChange = true
                   }
                }

                // DATE
                if !foundChange && !strings.Contains(trimmed, ":") && reSimpleDate.MatchString(trimmed) {
                   res := formatDate(trimmed, settings.Format)
                   if res != "" && res != cell {
                      currentResult = res
                      foundChange = true
                   }
                }

                // PRICE
                if !foundChange && rePotentialPrice.MatchString(trimmed) && reDigits.MatchString(trimmed) {
                   isSimpleID := !strings.ContainsAny(trimmed, ".,") && len(cleanDigits) <= 6 && !strings.Contains(strings.ToLower(trimmed), "руб")
                   if !isSimpleID {
                      res := formatNumber(trimmed, settings.Format)
                      if res != cell && res != "" {
                         currentResult = res
                         foundChange = true
                      }
                   }
                }

                if !foundChange && cell != trimmed {
                   currentResult = trimmed
                   foundChange = true
                }
             }
          }

          if foundChange {
             hasStartSpace := len(cell) > 0 && unicode.IsSpace(rune(cell[0]))
             hasEndSpace := len(cell) > 0 && unicode.IsSpace(rune(cell[len(cell)-1]))
             pre := ""; if hasStartSpace { pre = "\u00A0" }
             suf := ""; if hasEndSpace { suf = "\u00A0" }

             meta.Errors = append(meta.Errors, models.FileError{
                Row:         i + 1,
                Column:      rawColName,
                OldValue:    fmt.Sprintf("'%s%s%s'", pre, cell, suf),
                NewValue:    fmt.Sprintf("'%s'", currentResult),
                Description: rawColName,
                HandFormat:  wasManual,
             })

             if save {
                f.SetCellValue(sheet, excelAddr, nil)
                f.SetCellStyle(sheet, excelAddr, excelAddr, textStyle)
                f.SetCellStr(sheet, excelAddr, currentResult)
             }
          }
       }
    }

    if save {
       if err := f.SaveAs(meta.FilePath); err != nil {
          log.Printf("[API-LOG] ❌ Ошибка при сохранении файла: %v", err)
          return err
       }
       log.Printf("[API-LOG] ✅ ФАЙЛ УСПЕШНО ПЕРЕЗАПИСАН")
    }

    log.Printf("[API-LOG] <<< ОБРАБОТКА ЗАВЕРШЕНА. Найдено правок: %d", len(meta.Errors))
    return nil
}