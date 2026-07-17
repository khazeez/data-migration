package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	Database DatabaseConfig `yaml:"database"`
	Google   GoogleConfig   `yaml:"google"`
}

func (c *AppConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.Database.User, c.Database.Password,
		c.Database.Host, c.Database.Port, c.Database.DBName,
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

type TableConfig struct {
	Sheet    SheetConfig            `yaml:"sheet"`
	Table    string                 `yaml:"table"`
	Mapping  map[string]ColumnMap   `yaml:"mapping"`
	Defaults map[string]interface{} `yaml:"defaults"`
}

type SheetConfig struct {
	SpreadsheetID string `yaml:"spreadsheet_id"`
	Worksheet     string `yaml:"worksheet"`
}

type ColumnMap struct {
	Column   string `yaml:"column"`
	Required bool   `yaml:"required,omitempty"`
}

type JobConfig struct {
	Name   string   `yaml:"name"`
	Tables []string `yaml:"tables"`
}

func LoadApp(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read app config: %w", err)
	}
	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse app config: %w", err)
	}
	return &cfg, nil
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
