package service

import (
	"doctordoc/internal/models"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

func (s *fileService) processCSV(meta *models.FileMetadata, save bool, userReq models.FixRequest) error {
	log.Printf("\n[API-LOG] >>> СТАРТ ОБРАБОТКИ CSV. Файл: %s, Save: %v", meta.FilePath, save)
	log.Printf("[API-LOG] 📥 ПОЛУЧЕНО ОТ ФРОНТЕНДА (userReq): %+v", userReq)

	content, err := os.ReadFile(meta.FilePath)
	if err != nil {
		log.Printf("[API-LOG] ❌ Ошибка чтения файла: %v", err)
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
	reader.TrimLeadingSpace = false

	records, err := reader.ReadAll()
	if err != nil || len(records) == 0 {
		log.Printf("[API-LOG] ❌ Ошибка парсинга CSV: %v", err)
		return err
	}

	headers := records[0]
	log.Printf("[API-LOG] 📋 Заголовки в CSV: %v", headers)

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
		for reqColKey, settings := range userReq.Columns {
			reqClean := strings.ToLower(strings.TrimSpace(reqColKey))
			if reqClean == hClean || reqClean == fmt.Sprintf("%d", j) {
				colSettingsByIndex[j] = settings
				log.Printf("[API-LOG] ✅ CSV СВЯЗКА: Колонка %d (%s) <---> Правило '%s'", j, hName, reqColKey)
			}
		}
	}

	reEmail := regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
	reDigits := regexp.MustCompile(`\d`)
	reOnlyDigits := regexp.MustCompile(`\D`)
	reHasLetters := regexp.MustCompile(`(?i)[a-zа-яё]`)
	rePotentialPrice := regexp.MustCompile(`(?i)(\d+[\s.,]?\d*\s?(руб|р\.|\bрубл[яьей]\b|₽|\$|€|¥))`)
	reSimpleDate := regexp.MustCompile(`^(\d{1,4})[.,/\- ](\d{1,2})[.,/\- ](\d{1,4})`)

	selectedMap := make(map[int]bool)
	for _, r := range userReq.SelectedRows {
		selectedMap[r] = true
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	semaphore := make(chan struct{}, 50)

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
				if j >= len(headers) {
					continue
				}

				rawColName := headers[j]
				settings, hasSettings := colSettingsByIndex[j]

				cleanInput := strings.Map(func(r rune) rune {
					if unicode.IsSpace(r) {
						return ' '
					}
					return r
				}, cell)
				trimmed := strings.TrimSpace(cleanInput)

				log.Printf(
					"[CSV][CELL] row=%d col=%d (%s) raw='%s' trimmed='%s' settings=%+v",
					idx+1, j, rawColName, cell, trimmed, settings,
				)

				currentResult := cell
				foundChange := false
				wasManual := false

				if !hasSettings {
					if cell != trimmed {
						currentResult = trimmed
						foundChange = true
					}
				} else {
					if settings.Manual && settings.Format != "" {
						fLower := strings.ToLower(settings.Format)
						var manualRes string

						log.Printf("[CSV][MANUAL] col=%s format='%s'", rawColName, settings.Format)

						isDate := strings.ContainsAny(fLower, "ymd")
						isCurrency := strings.ContainsAny(fLower, "₽$€¥") ||
							regexp.MustCompile(`(?i)(руб\.?|\bрубл[яьей]\b|р\.)`).MatchString(fLower)
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

						log.Printf("[CSV][MANUAL][RESULT] before='%s' after='%s'", cell, manualRes)

						// 🔥 ФИКС — как в XLSX
						if manualRes != "" {
							currentResult = manualRes
							foundChange = true
							wasManual = true
						}
					}

					if settings.Auto && !foundChange && cell != "" {
						cleanDigits := reOnlyDigits.ReplaceAllString(trimmed, "")

						if strings.Contains(trimmed, "@") && reEmail.MatchString(strings.ReplaceAll(trimmed, " ", "")) {
							res := strings.ToLower(strings.ReplaceAll(trimmed, " ", ""))
							if res != cell {
								currentResult = res
								foundChange = true
							}
						}

						if !foundChange && len(cleanDigits) >= 10 && len(cleanDigits) <= 12 && !reHasLetters.MatchString(trimmed) {
							mask := "7XXXXXXXXXX"
							if settings.Format != "" {
								mask = settings.Format
							}

							log.Printf(
								"[CSV][AUTO][PHONE] val='%s' digits='%s' mask='%s'",
								trimmed, cleanDigits, mask,
							)

							res := formatPhone(trimmed, mask)

							log.Printf(
								"[CSV][AUTO][PHONE][RESULT] before='%s' after='%s'",
								cell, res,
							)

							if res != cell {
								currentResult = res
								foundChange = true
							}
						}

						if !foundChange && !strings.Contains(trimmed, ":") && reSimpleDate.MatchString(trimmed) {
							res := formatDate(trimmed, settings.Format)
							if res != "" && res != cell {
								currentResult = res
								foundChange = true
							}
						}

						if !foundChange && rePotentialPrice.MatchString(trimmed) && reDigits.MatchString(trimmed) {
							res := formatNumber(trimmed, settings.Format)
							if res != cell && res != "" {
								currentResult = res
								foundChange = true
							}
						}

						if !foundChange && cell != trimmed {
							currentResult = trimmed
							foundChange = true
						}
					}
				}

				if foundChange {
					mu.Lock()
					hasStartSpace := len(cell) > 0 && unicode.IsSpace(rune(cell[0]))
					hasEndSpace := len(cell) > 0 && unicode.IsSpace(rune(cell[len(cell)-1]))
					pre := ""
					if hasStartSpace {
						pre = "\u00A0"
					}
					suf := ""
					if hasEndSpace {
						suf = "\u00A0"
					}

					meta.Errors = append(meta.Errors, models.FileError{
						Row:         idx + 1,
						Column:      rawColName,
						OldValue:    fmt.Sprintf("'%s%s%s'", pre, cell, suf),
						NewValue:    fmt.Sprintf("'%s'", currentResult),
						Description: rawColName,
						HandFormat:  wasManual,
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
		if err != nil {
			return err
		}
		defer f.Close()
		w := csv.NewWriter(f)
		w.Comma = sep
		w.WriteAll(records)
		log.Printf("[API-LOG] ✅ CSV ПЕРЕЗАПИСАН")
	}

	log.Printf("[API-LOG] <<< ОБРАБОТКА CSV ЗАВЕРШЕНА. Найдено правок: %d", len(meta.Errors))
	return nil
}
