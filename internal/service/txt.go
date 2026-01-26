package service

import (
	"doctordoc/internal/models"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

func (s *fileService) processTXT(meta *models.FileMetadata, save bool, userReq models.FixRequest) error {
	log.Printf("[TXT-LOG] >>> ЗАПУСК ОБРАБОТКИ (Файл: %s, Save: %v)", meta.OriginalName, save)

	content, err := os.ReadFile(meta.FilePath)
	if err != nil {
		log.Printf("[TXT-LOG] ❌ Ошибка чтения: %v", err)
		return err
	}

	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Настройки для TXT
	settings, hasSettings := userReq.Columns["ТЕКСТ"]

	// Если это ПЕРВЫЙ запуск (анализ), принудительно включаем Auto,
	// иначе StartWorker всегда будет возвращать 0 ошибок.
	if !hasSettings {
		settings = models.ColumnSettings{Auto: true}
	}

	reEmail := regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`)
	reDigits := regexp.MustCompile(`\D`)
	reDateLike := regexp.MustCompile(`(\d{1,4})[./\s,\\\-_](\d{1,2})[./\s,\\\-_](\d{2,4})`)

	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" { continue }

		// Проверка выбора строк (только если это не первичный анализ)
		isRowSelected := true
		if hasSettings && len(userReq.SelectedRows) > 0 {
			isRowSelected = false
			for _, r := range userReq.SelectedRows {
				if r == i+1 { isRowSelected = true; break }
			}
		}
		if !isRowSelected { continue }

		wg.Add(1)
		go func(idx int, rawLine string) {
			defer wg.Done()

			res := strings.TrimSpace(rawLine)
			changed := false
			trimmed := strings.TrimSpace(rawLine)

			// 1. РУЧНОЙ ФОРМАТ (только если есть настройки от юзера)
			if hasSettings && settings.Manual && settings.Format != "" {
				if strings.Contains(settings.Format, "X") {
					res = formatPhone(rawLine, settings.Format)
				} else if strings.Contains(settings.Format, "yyyy") || strings.Contains(settings.Format, "dd") {
					res = formatDate(rawLine, settings.Format)
				} else if strings.Contains(settings.Format, "234") || settings.Format == "int" {
					res = formatNumber(rawLine, settings.Format)
				}
				if res != rawLine { changed = true }
			}

			// 2. АВТО-ПРАВКА (всегда работает при первом анализе или если включен Auto)
			if !changed && settings.Auto {
				// Email
				if strings.Contains(trimmed, "@") && reEmail.MatchString(strings.ReplaceAll(trimmed, " ", "")) {
					res = strings.ToLower(strings.ReplaceAll(trimmed, " ", ""))
					changed = res != rawLine
				}
				// Дата
				if !changed {
					if m := reDateLike.FindStringSubmatch(trimmed); m != nil {
						d, mo, y := m[1], m[2], m[3]
						if len(d) == 1 { d = "0" + d }; if len(mo) == 1 { mo = "0" + mo }; if len(y) == 2 { y = "20" + y }
						if t, err := time.Parse("02.01.2006", d+"."+mo+"."+y); err == nil {
							res = t.Format("02.01.2006")
							changed = res != rawLine
						}
					}
				}
				// Телефон
				if !changed {
					cd := reDigits.ReplaceAllString(trimmed, "")
					if len(cd) >= 10 && len(cd) <= 12 {
						res = "7" + cd[len(cd)-10:]
						changed = res != rawLine
					}
				}
				// Просто Trim
				if !changed && rawLine != trimmed {
					res = trimmed
					changed = true
				}
			}

			if changed {
				mu.Lock()
				log.Printf("[TXT-FIX] Строка %d: '%s' -> '%s'", idx+1, rawLine, res)
				meta.Errors = append(meta.Errors, models.FileError{
					Row:         idx + 1,
					Column:      "ТЕКСТ",
					OldValue:    rawLine,
					NewValue:    res,
					Description: "Найдены неточности в тексте",
				})
				if save { lines[idx] = res }
				mu.Unlock()
			}
		}(i, line)
	}
	wg.Wait()

	if save {
		os.WriteFile(meta.FilePath, []byte(strings.Join(lines, "\n")), 0644)
		log.Printf("[TXT-LOG] ✅ Файл перезаписан")
	}
	return nil
}