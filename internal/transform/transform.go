package transform

import (
	"fmt"
	"strings"

	"data-migration/internal/config"
)

type Transformer struct {
	tableCfg *config.TableConfig
}

func New(tableCfg *config.TableConfig) *Transformer {
	return &Transformer{tableCfg: tableCfg}
}

type RowResult struct {
	Values map[string]interface{}
}

func (t *Transformer) TransformRow(headerIdx map[string]int, row []interface{}) (*RowResult, error) {
	result := &RowResult{Values: make(map[string]interface{}, len(t.tableCfg.Mapping))}

	for sheetCol, colMap := range t.tableCfg.Mapping {
		idx, ok := headerIdx[sheetCol]
		if !ok {
			if colMap.Required {
				return nil, fmt.Errorf("required column %q not found in sheet", sheetCol)
			}
			continue
		}

		var val interface{}
		if idx < len(row) && row[idx] != nil {
			val = row[idx]
		}

		if val == nil || fmt.Sprintf("%v", val) == "" {
			if def, ok := t.tableCfg.Defaults[colMap.Column]; ok {
				val = resolveDefault(def)
			}
		}

		if colMap.Transform != "" && val != nil {
			val = applyTransform(val, colMap.Transform)
		}

		result.Values[colMap.Column] = val
	}

	for col, def := range t.tableCfg.Defaults {
		if _, exists := result.Values[col]; !exists || result.Values[col] == nil {
			result.Values[col] = resolveDefault(def)
		}
	}

	return result, nil
}

func (t *Transformer) BuildColumnsAndRows(headerIdx map[string]int, sheetRows [][]interface{}) ([]string, [][]interface{}, error) {
	columns := make([]string, 0, len(t.tableCfg.Mapping))
	colSet := make(map[string]bool, len(t.tableCfg.Mapping))

	for _, cm := range t.tableCfg.Mapping {
		if !colSet[cm.Column] {
			columns = append(columns, cm.Column)
			colSet[cm.Column] = true
		}
	}

	for col := range t.tableCfg.Defaults {
		if !colSet[col] {
			columns = append(columns, col)
			colSet[col] = true
		}
	}

	var rows [][]interface{}
	for _, row := range sheetRows {
		result, err := t.TransformRow(headerIdx, row)
		if err != nil {
			return nil, nil, fmt.Errorf("transform row: %w", err)
		}

		values := make([]interface{}, len(columns))
		for i, col := range columns {
			if v, ok := result.Values[col]; ok {
				values[i] = v
			} else {
				values[i] = nil
			}
		}
		rows = append(rows, values)
	}

	return columns, rows, nil
}

func applyTransform(val interface{}, transform string) interface{} {
	s := strings.TrimSpace(fmt.Sprintf("%v", val))
	switch transform {
	case "string_to_bool":
		switch strings.ToLower(s) {
		case "y", "yes", "1", "true", "t", "active", "deployed":
			return true
		case "n", "no", "0", "false", "f", "inactive", "undeployed":
			return false
		default:
			return val
		}
	case "lower":
		return strings.ToLower(s)
	case "upper":
		return strings.ToUpper(s)
	case "trim":
		return strings.TrimSpace(s)
	default:
		return val
	}
}

func resolveDefault(def interface{}) interface{} {
	s, ok := def.(string)
	if !ok {
		return def
	}
	switch strings.ToLower(s) {
	case "uuid":
		return uuidV4()
	case "now":
		return nowString()
	case "null":
		return nil
	default:
		return def
	}
}
