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
    norm := regexp.MustCompile(`[/-]`).ReplaceAllString(val, ".")
    layouts := []string{"02.01.2006", "02.01.06", "2006.01.02", "01.02.2006", "02.01.2006 15:04"}

    var t time.Time
    found := false
    for _, l := range layouts {
       if parsed, err := time.Parse(l, norm); err == nil {
          t = parsed
          found = true
          break
       }
    }
    if !found {
        log.Printf("[GO/Date] ❌ Could not parse date: %s", val)
        return val
    }

    f := customMask
    f = strings.ReplaceAll(f, "yyyy", "2006")
    f = strings.ReplaceAll(f, "yy", "06")
    f = strings.ReplaceAll(f, "MM", "01")
    f = strings.ReplaceAll(f, "dd", "02")
    f = strings.ReplaceAll(f, "HH", "15")
    f = strings.ReplaceAll(f, "mm", "04")
    f = strings.ReplaceAll(f, "ss", "05")

    res := t.Format(f)
    log.Printf("[GO/Date] ✅ Formatted %s -> %s using %s", val, res, f)
    return res
}

func formatNumber(val, mask string) string {
    if val == "" { return "" }
    clean := regexp.MustCompile(`[^0-9,.-]`).ReplaceAllString(val, "")
    clean = strings.ReplaceAll(clean, ",", ".")
    num, err := strconv.ParseFloat(clean, 64)
    if err != nil {
        log.Printf("[GO/Num] ❌ Could not parse number: %s", val)
        return val
    }

    var result string
    if mask == "int" {
        result = fmt.Sprintf("%.0f", num)
    } else {
        thousandSep := ""
        if strings.Contains(mask, "1,234") { thousandSep = "," }
        if strings.Contains(mask, "1 234") { thousandSep = " " }

        decimalSep := "."
        if strings.Contains(mask, "34,56") { decimalSep = "," }

        res := fmt.Sprintf("%.2f", num)
        parts := strings.Split(res, ".")
        intP, decP := parts[0], parts[1]

        var withSep string
        for i, v := range intP {
           if i > 0 && (len(intP)-i)%3 == 0 && thousandSep != "" {
              withSep += thousandSep
           }
           withSep += string(v)
        }
        result = withSep + decimalSep + decP
    }

    log.Printf("[GO/Num] ✅ Formatted %s -> %s (Mask: %s)", val, result, mask)
    return result
}