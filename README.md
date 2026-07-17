# sheet-loader

Load data from Google Sheets into PostgreSQL tables using YAML-based configuration.

## Features

- Google Sheets API integration
- YAML configuration for jobs, tables, and column mapping
- Auto UUID generation
- Default values (uuid, now, null, etc.)
- Batch insert with configurable batch size
- Transaction with automatic rollback on failure
- Dry-run mode for validation
- Header validation with missing/unmapped column reporting
- Progress logging with timing
- Truncate-before-insert strategy

## Project Structure

```
├── cmd/main.go              # Entry point
├── configs/
│   ├── app.yml              # App configuration (DB, Google credentials)
│   ├── jobs/                # Job definitions
│   └── tables/              # Per-table mapping configs
├── internal/
│   ├── config/              # Configuration loader
│   ├── db/                  # PostgreSQL connection & batch insert
│   ├── importer/            # Import orchestration
│   ├── jobs/                # Job runner
│   ├── logger/              # Structured logger
│   ├── mapper/              # Column index mapping
│   ├── sheets/              # Google Sheets client
│   ├── transform/           # Data transformation & defaults
│   ├── utils/               # Utilities (UUID, defaults)
│   └── validator/           # Header validation
├── credentials.json         # Google service account credentials
├── go.mod
├── Makefile
└── README.md
```

## Prerequisites

- Go 1.21+
- PostgreSQL
- Google Cloud service account with Sheets API enabled
- `credentials.json` from Google Cloud Console

## Setup

1. Clone the repository
2. Place your Google service account `credentials.json` in the project root
3. Edit `configs/app.yml` with your PostgreSQL connection details
4. Configure your sheet-to-table mappings in `configs/tables/`
5. Configure your job in `configs/jobs/`

## Usage

```bash
# List available jobs
go run ./cmd/ -list

# Dry run (validate without inserting)
go run ./cmd/ -job master-data -dry-run

# Run a job
go run ./cmd/ -job master-data

# With custom config path
go run ./cmd/ -job master-data -config ./configs/app.yml

# Custom batch size and log level
go run ./cmd/ -job master-data -batch 1000 -log-level debug
```

Or using Make:

```bash
make list
make dry-run
make run
```

## Configuration

### app.yml

```yaml
database:
  host: localhost
  port: 5432
  user: postgres
  password: postgres
  dbname: master

google:
  credential: credentials.json
```

### Table config (`configs/tables/role.yml`)

```yaml
sheet:
  spreadsheet_id: YOUR_SPREADSHEET_ID
  worksheet: Role

table: m_role

mapping:
  Role Name:
    column: name
    required: true
  Description:
    column: description

defaults:
  id: uuid
  active: true
  created_at: now
  updated_at: now
  created_by: system
```

### Default value types

| Value   | Description               |
|---------|---------------------------|
| `uuid`  | Auto-generated UUID v4    |
| `now`   | Current UTC timestamp     |
| `null`  | NULL value                |
| `true`  | Boolean true              |
| `false` | Boolean false             |
| other   | Used as-is                |

## Flags

| Flag         | Default              | Description                  |
|--------------|----------------------|------------------------------|
| `-config`    | `configs/app.yml`    | Path to app config file      |
| `-job`       | (required)           | Job name to execute          |
| `-list`      | `false`              | List available jobs          |
| `-dry-run`   | `false`              | Validate without inserting   |
| `-batch`     | `500`                | Batch insert size            |
| `-log-level` | `info`               | Log level (debug/info/warn/error) |
