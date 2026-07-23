package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	Database DatabaseConfig `yaml:"database"`
	Google   GoogleConfig   `yaml:"google"`
}

// DSN mengembalikan connection string format key=value (dipakai oleh sebagian driver).
func (c *AppConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		c.Database.Host, c.Database.Port,
		c.Database.User, c.Database.Password,
		c.Database.DBName,
	)
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
}

type GoogleConfig struct {
	Credential string `yaml:"credential"`
}

type ConflictConfig struct {
	Action string   `yaml:"action"`
	Keys   []string `yaml:"keys"`
}

type FilterConfig struct {
	Column string        `yaml:"column"`
	Value  interface{}   `yaml:"value,omitempty"`
	Values []interface{} `yaml:"values,omitempty"`
}

type TableConfig struct {
	Sheet      SheetConfig            `yaml:"sheet"`
	Table      string                 `yaml:"table"`
	Mapping    map[string]ColumnMap   `yaml:"mapping"`
	Defaults   map[string]interface{} `yaml:"defaults"`
	OnConflict *ConflictConfig        `yaml:"on_conflict,omitempty"`
	Filter     *FilterConfig          `yaml:"filter,omitempty"`
	Filters    []FilterConfig         `yaml:"filters,omitempty"`
	Unique     []string               `yaml:"unique,omitempty"`
}

type SheetConfig struct {
	SpreadsheetID string `yaml:"spreadsheet_id"`
	Worksheet     string `yaml:"worksheet"`
}

type LookupConfig struct {
	Table string `yaml:"table"`
	From  string `yaml:"from"`
	To    string `yaml:"to"`
}

type ColumnMap struct {
	Column    string        `yaml:"column"`
	Required  bool          `yaml:"required,omitempty"`
	Transform string        `yaml:"transform,omitempty"`
	Lookup    *LookupConfig `yaml:"lookup,omitempty"`
}

type JobConfig struct {
	Name   string   `yaml:"name"`
	Tables []string `yaml:"tables,omitempty"`
	Jobs   []string `yaml:"jobs,omitempty"`
}

func LoadApp(path string) (*AppConfig, error) {
	var cfg AppConfig
	if path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("parse app config: %w", err)
			}
		}
	}
	applyEnvOverrides(&cfg)
	return &cfg, nil
}

func applyEnvOverrides(cfg *AppConfig) {
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Database.Port = p
		}
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.Database.DBName = v
	}
	if v := os.Getenv("GOOGLE_CREDENTIAL"); v != "" {
		cfg.Google.Credential = v
	}
}

func LoadTable(path string) (*TableConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read table config %s: %w", path, err)
	}
	var cfg TableConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse table config %s: %w", path, err)
	}
	if cfg.Sheet.Worksheet == "" {
		cfg.Sheet.Worksheet = cfg.Table
	}
	return &cfg, nil
}

func LoadJob(path string) (*JobConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read job config %s: %w", path, err)
	}
	var cfg JobConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse job config %s: %w", path, err)
	}
	return &cfg, nil
}

func ResolveTablePaths(tablesDir string, tableNames []string) []string {
	var paths []string
	for _, name := range tableNames {
		paths = append(paths, filepath.Join(tablesDir, name+".yml"))
	}
	return paths
}
