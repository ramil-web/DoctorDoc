package service

import (
    "fmt"
    "log"
    "regexp"
    "strconv"
    "strings"
    "time"
)

func formatDate(val, customMask string) string {
    if val == "" {
       return ""
    }

    norm := regexp.MustCompile(`[/-]`).ReplaceAllString(val, ".")
    norm = strings.ReplaceAll(norm, ",", ".")
    norm = strings.TrimSpace(norm)

    layouts := []string{
       "02.01.2006", "02.01.06", "2006.01.02", "01.02.2006",
       "02.01.2006 15:04", "2006-01-02", "2.1.2006", "30,01,2023",
    }

    var t time.Time
    found := false
    for _, l := range layouts {
       lNorm := strings.ReplaceAll(l, ",", ".")
       if parsed, err := time.Parse(lNorm, norm); err == nil {
          t = parsed
          found = true
          break
       }
    }

    if !found {
       log.Printf("[DEBUG/Date] ❌ Не удалось распознать дату: %s", val)
       return val
    }

    f := customMask
    if f == "" || (!strings.ContainsAny(strings.ToLower(f), "ymd") && !strings.Contains(f, "May")) {
       f = "02.01.2006"
    }

    f = strings.ReplaceAll(f, "yyyy", "2006")
    f = strings.ReplaceAll(f, "yy", "06")
    f = strings.ReplaceAll(f, "MMMM", "January")
    f = strings.ReplaceAll(f, "MMM", "Jan")
    f = strings.ReplaceAll(f, "MM", "01")
    f = strings.ReplaceAll(f, "dd", "02")
    f = strings.ReplaceAll(f, "HH", "15")
    f = strings.ReplaceAll(f, "mm", "04")
    f = strings.ReplaceAll(f, "ss", "05")

    res := t.Format(f)
    log.Printf("[DEBUG/Date] ✅ %s -> %s (Маска: %s)", val, res, f)
    return res
}

func formatNumber(val, mask string) string {
    if val == "" {
        return ""
    }

    // Запоминаем оригинальный символ валюты из ячейки
    origCurrency := ""
    valLower := strings.ToLower(val)
    if strings.Contains(valLower, " руб") {
        origCurrency = " руб"
    } else if strings.Contains(valLower, " р") {
        origCurrency = " р."
    } else if strings.Contains(val, "₽") {
        origCurrency = " ₽"
    }

    reNum := regexp.MustCompile(`[0-9.,-]+`)
    numStr := reNum.FindString(val)
    if numStr == "" {
        return val
    }

    clean := strings.ReplaceAll(numStr, ",", ".")
    num, err := strconv.ParseFloat(clean, 64)
    if err != nil {
        return val
    }

    precision := 0
    needsPrecision := strings.Contains(mask, ".00") ||
                      strings.Contains(mask, ",00") ||
                      strings.Contains(mask, ",XX") ||
                      strings.Contains(mask, ".XX") ||
                      strings.Contains(mask, ",56")

    if needsPrecision {
        precision = 2
    }

    thousandSep := ""
    if strings.Contains(mask, " ") || strings.Contains(mask, "X X") || mask == "" {
        thousandSep = " "
    } else if strings.Contains(mask, ",") && !strings.Contains(mask, ",X") && !needsPrecision {
        thousandSep = ","
    }

    decimalSep := "."
    if strings.Contains(mask, ",XX") || strings.Contains(mask, ",56") || strings.Contains(mask, ",00") {
        decimalSep = ","
    }

    formatStr := fmt.Sprintf("%%.%df", precision)
    res := fmt.Sprintf(formatStr, num)

    parts := strings.Split(res, ".")
    intP := parts[0]

    var withSep string
    for i, v := range intP {
        if i > 0 && (len(intP)-i)%3 == 0 && thousandSep != "" {
            withSep += thousandSep
        }
        withSep += string(v)
    }

    result := withSep
    if precision > 0 && len(parts) > 1 {
        result += decimalSep + parts[1]
    }

    if mask != "" && strings.Contains(mask, "X") {
        // Если в маске есть X, заменяем блок иксов на число.
        // Символы руб/р/₽ возьмутся из самой маски (например "XXXX руб")
        reX := regexp.MustCompile(`X[X\s,.]*X|X`)
        result = reX.ReplaceAllString(mask, result)
    } else if origCurrency != "" {
        // Если маски нет, но в оригинале была валюта — возвращаем её как было
        if !strings.Contains(result, strings.TrimSpace(origCurrency)) {
            result = result + origCurrency
        }
    }

    log.Printf("[DEBUG/Num] Итог: %s", result)
    return result
}

func formatPhone(val, mask string) string {
    if val == "" {
       return ""
    }

    re := regexp.MustCompile(`\D`)
    digits := re.ReplaceAllString(val, "")

    if len(digits) < 10 {
       log.Printf("[DEBUG/Phone] ⚠️ Слишком мало цифр (%d): %s", len(digits), val)
       return val
    }

    // Берём последние 10 цифр (хвост)
    pure := digits[len(digits)-10:]
    log.Printf("[DEBUG/Phone] Вход: %s, Маска: %s, Хвост: %s", val, mask, pure)

    // ИСПРАВЛЕНО: Жёсткое определение префикса из начала маски
    currentPrefix := ""
    if strings.HasPrefix(mask, "+7") {
        currentPrefix = "+7"
    } else if strings.HasPrefix(mask, "8") {
        currentPrefix = "8"
    } else if strings.HasPrefix(mask, "7") {
        currentPrefix = "7"
    } else {
        // Если в маске нет префикса, по дефолту 7
        currentPrefix = "7"
    }

    log.Printf("[DEBUG/Phone] Выбран префикс: %s", currentPrefix)

    // Если маска без иксов (просто 79991112233), делаем её шаблоном
    if !strings.Contains(mask, "X") && len(re.ReplaceAllString(mask, "")) >= 10 {
        mask = currentPrefix + " (XXX) XXX-XX-XX"
        log.Printf("[DEBUG/Phone] Маска стала шаблоном: %s", mask)
    }

    if !strings.Contains(mask, "X") {
       res := currentPrefix + pure
       log.Printf("[DEBUG/Phone] Итог (без X): %s", res)
       return res
    }

    result := ""
    digitIdx := 0
    // Находим место, где начинаются иксы для хвоста
    // Мы игнорируем префикс маски, если он уже есть (7, 8 или +7)

    // Чтобы не дублировать префикс, если он зашит в маске как не-X:
    for _, char := range mask {
       if char == 'X' {
          if digitIdx < len(pure) {
             result += string(pure[digitIdx])
             digitIdx++
          }
       } else {
          result += string(char)
       }
    }

    log.Printf("[DEBUG/Phone] Итог: %s", result)
    return result
}
