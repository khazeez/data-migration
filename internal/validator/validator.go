package validator

import (
	"fmt"
	"strings"

	"data-migration/internal/config"
)

type ValidationReport struct {
	Valid       bool
	Missing     []string
	Unmapped    []string
	TableConfig string
}

func ValidateHeaders(sheetHeaders []string, tableCfg *config.TableConfig) *ValidationReport {
	report := &ValidationReport{
		Valid:       true,
		TableConfig: tableCfg.Table,
	}

	sheetSet := make(map[string]bool, len(sheetHeaders))
	for _, h := range sheetHeaders {
		sheetSet[strings.TrimSpace(h)] = true
	}

	for sheetCol, colMap := range tableCfg.Mapping {
		if !sheetSet[sheetCol] && colMap.Required {
			report.Valid = false
			report.Missing = append(report.Missing, sheetCol)
		}
	}

	for _, h := range sheetHeaders {
		trimmed := strings.TrimSpace(h)
		if _, ok := tableCfg.Mapping[trimmed]; !ok {
			report.Unmapped = append(report.Unmapped, trimmed)
		}
	}

	return report
}

func (r *ValidationReport) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Table: %s\n", r.TableConfig))
	if r.Valid {
		b.WriteString("  Headers: OK\n")
	} else {
		b.WriteString("  Headers: INVALID\n")
	}
	if len(r.Missing) > 0 {
		b.WriteString(fmt.Sprintf("  Missing required: %s\n", strings.Join(r.Missing, ", ")))
	}
	if len(r.Unmapped) > 0 {
		b.WriteString(fmt.Sprintf("  Unmapped columns: %s\n", strings.Join(r.Unmapped, ", ")))
	}
	return b.String()
}
