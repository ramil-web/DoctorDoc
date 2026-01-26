package service

import (
	"doctordoc/internal/models"
	"fmt"
	"github.com/xuri/excelize/v2"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode"
)

func (s *fileService) processXLSX(meta *models.FileMetadata, save bool, userReq models.FixRequest) error {
	log.Printf("\n[API-LOG] >>> СТАРТ ОБРАБОТКИ. Сохранение: %v", save)

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
				log.Printf("[API-LOG] ✅ СВЯЗКА: Колонка %d (%s) <---> '%s'", j, excelLetter, reqColKey)
			}
		}
	}

	meta.Errors = []models.FileError{}
	reEmail := regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
	reDigits := regexp.MustCompile(`\d`)
	reOnlyDigits := regexp.MustCompile(`\D`)
	// Улучшенное распознавание цены: наличие спец-символов ИЛИ просто поток цифр (сумма)
	rePotentialPrice := regexp.MustCompile(`(?i)(руб|р\.|\d+,\d+|^\s*\d[\d\s]{2,8}\s*$)`)
	reSimpleDate := regexp.MustCompile(`^(\d{1,4})[/-](\d{1,2})[/-](\d{1,4})$`)
	textStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 49})

	for i, row := range rows {
		if i == 0 { continue }

		for j, cell := range row {
			if j >= len(headers) { continue }

			rawColName := headers[j]
			excelAddr, _ := excelize.CoordinatesToCellName(j+1, i+1)
			settings, hasSettings := colSettingsByIndex[j]

			cleanInput := strings.Map(func(r rune) rune {
				if unicode.IsSpace(r) { return ' ' }
				return r
			}, cell)
			trimmed := strings.TrimSpace(cleanInput)

			currentResult := cell
			foundChange := false

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
					if strings.Contains(fLower, "x") {
						manualRes = formatPhone(cell, settings.Format)
					} else if strings.Contains(fLower, "y") || strings.Contains(fLower, "d") {
						manualRes = formatDate(cell, settings.Format)
					} else if strings.Contains(fLower, "234") || fLower == "int" {
						manualRes = formatNumber(cell, settings.Format)
					}

					if manualRes != "" && manualRes != cell {
						currentResult = manualRes
						foundChange = true
					}
				}

				// 2. АВТО ФОРМАТИРОВАНИЕ
				if settings.Auto && !foundChange && cell != "" {
					cleanDigits := reOnlyDigits.ReplaceAllString(trimmed, "")

					// EMAIL
					if strings.Contains(trimmed, "@") && reEmail.MatchString(strings.ReplaceAll(trimmed, " ", "")) {
						res := strings.ToLower(strings.ReplaceAll(trimmed, " ", ""))
						if res != cell {
							currentResult = res
							foundChange = true
						}
					}

					// PHONE
					if !foundChange && len(cleanDigits) >= 10 && len(cleanDigits) <= 12 {
						mask := "7XXXXXXXXXX"
						if settings.Format != "" && strings.Contains(strings.ToLower(settings.Format), "x") {
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
						norm := regexp.MustCompile(`[/-]`).ReplaceAllString(trimmed, ".")
						for _, l := range []string{"02.01.2006", "2006.01.02", "02.01.06", "2.1.2006"} {
							if t, err := time.Parse(l, norm); err == nil {
								res := t.Format("02.01.2006")
								if res != cell {
									currentResult = res
									foundChange = true
								}
								break
							}
						}
					}

					// PRICE/AMOUNT (Улучшено)
					if !foundChange && rePotentialPrice.MatchString(trimmed) && reDigits.MatchString(trimmed) {
						res := formatNumber(trimmed, "")
						if res != cell && res != "" {
							currentResult = res
							foundChange = true
						}
					}

					// DEFAULT TRIM
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

				oldValDisplay := fmt.Sprintf("'%s%s%s'", pre, cell, suf)
				newValDisplay := fmt.Sprintf("'%s%s%s'", pre, currentResult, suf)

				meta.Errors = append(meta.Errors, models.FileError{
					Row: i + 1, Column: rawColName, OldValue: oldValDisplay, NewValue: newValDisplay, Description: rawColName,
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
		if err := f.SaveAs(meta.FilePath); err != nil { return err }
		log.Printf("[API-LOG] ✅ ФАЙЛ УСПЕШНО ПЕРЕЗАПИСАН")
	}
	return nil
}