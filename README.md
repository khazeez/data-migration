# sheet-loader

Tool migrasi data dari Google Sheets ke PostgreSQL menggunakan Go. Dikonfigurasi via YAML dan environment variable.

## Fitur

- **Integrasi Google Sheets API** — baca data dari spreadsheet
- **Mapping kolom** — mapping sheet column → DB column dengan rename
- **Transform value**: `string_to_bool`, `lower`, `upper`, `trim`
- **Lookup transform** — lookup value dari tabel lain (contoh: nama → ID)
- **Filter baris** — filter berdasarkan nilai kolom (case-insensitive)
- **Multi-filter** — multiple filter conditions dengan AND logic
- **Multi-value filter** — filter dengan multiple values (OR)
- **Not filter** — exclude nilai tertentu (`not: true`)
- **Not empty filter** — buang baris dengan kolom kosong (`not_empty: true`)
- **Default value otomatis**: `uuid`, `now`, `null`, `true`/`false`
- **Unique dedup** — hapus duplikat dari sheet data sebelum insert
- **On-conflict DB dedup** — query existing rows di DB, skip duplikat (tanpa perlu unique index)
- **Batch insert** dengan transaction dan rollback otomatis
- **Meta-job** — job yang menjalankan sub-job berurutan
- **Dry-run mode** — validasi tanpa insert
- **Pipeline stats** — log detail per table: raw → filtered → transformed → deduped → existing → inserted
- **Case-insensitive matching** — filter column name dan value
- **Whitespace trimming** — trim spasi dari cell value dan filter value
- **Cell overflow guard** — handle baris dengan lebih banyak cell dari header
- **UUID handling** — handle `[16]uint8` dan `pgtype.UUID` dari pgx
- **Semua konfigurasi via environment variable** — tanpa file `app.yml`
- **Taskfile** dengan auto-load `.env`

## Prerequisites

- Go 1.21+
- PostgreSQL
- Google Cloud service account dengan Sheets API enabled
- `credentials.json` dari Google Cloud Console
- Task CLI (optional, untuk `task` commands)

## Project Structure

```
├── cmd/main.go                    # Entry point CLI
├── configs/
│   ├── jobs/                      # Definisi job (daftar table)
│   │   ├── _all.yml               # Master job (run-all)
│   │   ├── consumer.yml
│   │   ├── group-endpoint.yml
│   │   ├── http.yml
│   │   ├── javascript.yml
│   │   ├── req-val.yml
│   │   └── sql.yml
│   └── tables/                    # Konfigurasi per-table
│       ├── consumer/
│       ├── group-endpoint/
│       ├── http/
│       ├── javascript/
│       └── request-validation/
├── internal/
│   ├── config/                    # Loader konfigurasi & structs
│   ├── db/                        # Koneksi PostgreSQL & batch insert
│   ├── importer/                  # Orchestrator: filter → transform → lookup → insert
│   ├── jobs/                      # Job runner (meta-job support)
│   ├── logger/                    # Logger
│   ├── mapper/                    # Index mapping kolom
│   ├── sheets/                    # Google Sheets client
│   ├── transform/                 # Transform data & default values
│   ├── utils/                     # Utility (UUID)
│   └── validator/                 # Validasi header sheet
├── credentials.json               # Service account key (di-gitignore)
├── .env                           # Environment variables (di-gitignore)
├── .env.example                   # Template env var
├── Taskfile.yml                   # Task runner
└── README.md
```

## Quick Start

### 1. Setup Credentials & Env

Copy `.env.example` ke `.env`, isi koneksi PostgreSQL dan path credentials Google.

### 2. Buat Table Config

```yaml
# configs/tables/customer.yml
sheet:
  spreadsheet_id: 1ABCxyz...
  worksheet: Customer Data

table: customers

mapping:
  Nama:
    column: name
  Status:
    column: is_active
    transform: string_to_bool
  Group:
    column: group_id
    transform: lookup
    lookup:
      table: groups
      from: name
      to: id

defaults:
  id: uuid
  created_at: now
  active: true

filter:
  column: Status
  value: active

unique: [name]

on_conflict:
  action: ignore
  keys: [name]
```

### 3. Buat Job Config

```yaml
# configs/jobs/master-data.yml
name: master-data
tables:
  - customer
```

### 4. Jalankan

```bash
# Dry-run
task dry-run -- master-data

# Migrasi
task run -- master-data
```

## Konfigurasi Table

### Sheet Config

```yaml
sheet:
  spreadsheet_id: 1ABCxyz...    # ID Google Sheet
  worksheet: Sheet1             # Nama worksheet/tab
```

### Mapping

Mapping sheet column ke DB column:

```yaml
mapping:
  Nama Lengkap:        # Kolom di sheet
    column: name       # Kolom di database
    required: true     # (optional) Wajib ada di sheet
  Status:
    column: is_active
    transform: string_to_bool   # Transform value
```

### Transform Values

| Transform | Input Contoh | Output |
|-----------|-------------|--------|
| `string_to_bool` | "Y", "yes", "1", "true", "t", "active", "deployed" | `true` |
| `string_to_bool` | "N", "no", "0", "false", "f", "inactive", "undeployed" | `false` |
| `lower` | "Hello World" | "hello world" |
| `upper` | "Hello World" | "HELLO WORLD" |
| `trim` | " value " | "value" |
| `lookup` | (lihat lookup section) | |

### Lookup Transform

Mengganti value sheet dengan ID dari tabel referensi di database:

```yaml
mapping:
  Nama Group:
    column: group_id
    transform: lookup
    lookup:
      table: groups         # Tabel referensi
      from: name            # Kolom pencarian (sheet value dicocokkan dengan ini)
      to: id                # Kolom hasil (nilai dari sini yang akan dipakai)
```

Flow: sheet value `"Admin"` → query `SELECT id FROM groups WHERE name = 'Admin'` → hasil `"uuid-admin"` dipakai sebagai `group_id`.

### Default Values

```yaml
defaults:
  id: uuid                  # Generate UUID v4
  created_at: now           # Timestamp current time (RFC3339)
  updated_at: now
  active: true              # Boolean true
  deleted_at: null          # Nilai null
  counter: 0                # Angka 0
```

### Filter

Filter menjalankan pada **data mentah dari sheet** (sebelum transform/mapping). Column name dan value dicocokkan secara **case-insensitive**.

#### Single filter

```yaml
filter:
  column: Status
  value: active           # Hanya baris dengan Status = "active"
```

#### Multiple values (OR)

```yaml
filter:
  column: type
  values: [oauth2, apikey]    # Hanya baris dengan type = oauth2 ATAU apikey
```

#### Not empty

```yaml
filter:
  column: Email
  not_empty: true       # Hanya baris yang kolom Email tidak kosong
```

#### Not / exclude

```yaml
filter:
  column: Status
  value: pending
  not: true             # Hanya baris dengan Status != "pending"
```

#### Multiple filters (AND)

```yaml
filters:
  - column: type
    values: [oauth2, apikey]
  - column: Email
    not_empty: true
  - column: Status
    value: pending
    not: true
```

Semua kondisi harus terpenuhi (AND). Nilai dalam `values` bersifat OR.

### Unique Dedup (Sheet Level)

Hapus baris duplikat dari data sheet berdasarkan kolom tertentu. Hanya baris pertama yang di-keep:

```yaml
unique: [name]           # Hapus duplikat berdasarkan kolom name
unique: [name, email]    # Kombinasi name + email
```

### On Conflict (DB Level)

Dedup dengan mengecek data yang sudah ada di database.

#### Ignore (app-level)

```yaml
on_conflict:
  action: ignore
  keys: [name]
```

Aplikasi akan query `SELECT DISTINCT name FROM table WHERE name IN (...)` lalu skip baris yang sudah ada. Tidak memerlukan unique index di PostgreSQL.

### Meta-Job

Job yang menjalankan sub-job berurutan:

```yaml
# configs/jobs/_all.yml
name: all
jobs:
  - consumer
  - req-val
  - javascript
  - http
  - group-endpoint
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-job` | (required) | Nama job dari `configs/jobs/` |
| `-dry-run` | `false` | Validasi tanpa insert |
| `-all` | `false` | Jalankan job `_all` (meta-job) |
| `-batch` | `500` | Ukuran batch insert |
| `-log-level` | `info` | Level log: `debug`, `info`, `warn`, `error` |
| `-list` | `false` | List available jobs |
| `-config` | `""` | Path ke app config (optional) |

Job name bisa via `-job` flag atau positional arg:

```bash
go run ./cmd/ -job consumer
go run ./cmd/ consumer           # positional
go run ./cmd/ -all               # run _all job
```

## Taskfile Commands

```bash
task run -- <job>         # Jalankan migrasi
task dry-run -- <job>     # Dry-run mode
task run-all              # Jalankan semua job (via _all)
task list                 # List available jobs
task build                # Build binary
task clean                # Hapus binary
```

Gunakan `--` untuk oper argumen:

```bash
task run -- consumer
task dry-run -- consumer
task run -- consumer -log-level debug -batch 200
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DB_HOST` | ✓ | - | Host PostgreSQL |
| `DB_PORT` | ✓ | `5432` | Port PostgreSQL |
| `DB_USER` | ✓ | - | User PostgreSQL |
| `DB_PASSWORD` | ✓ | - | Password PostgreSQL |
| `DB_NAME` | ✓ | - | Nama database |
| `GOOGLE_CREDENTIAL` | ✓ | - | Path ke credentials.json |
| `JOBS_DIR` | - | `configs/jobs` | Directory job config |
| `TABLES_DIR` | - | `configs/tables` | Directory table config |

Semua env variable otomatis di-load dari file `.env` oleh Taskfile.

## Alur Migrasi

Setiap table diproses dengan pipeline:

```
Raw Sheet → Filter → Transform + Lookup → Unique Dedup → DB Dedup → Batch Insert
```

Detailed flow:

1. Baca konfigurasi table dari YAML
2. Baca data dari Google Sheets API
3. Validasi header sheet vs mapping
4. **Filter** — buang baris yang tidak sesuai kondisi filter (case-insensitive, trim whitespace)
5. **Transform** — mapping kolom, transform value (string_to_bool, lower, dll), default values
6. **Lookup** — resolve lookup values dari database (batch query)
7. **Unique dedup** — hapus baris duplikat dari data sheet berdasarkan `unique` key
8. **DB dedup** (on_conflict ignore) — query existing rows, skip duplikat
9. **Batch insert** — insert dalam batch dengan transaction
10. Commit atau rollback jika error

Pipeline stats di-log setiap selesai satu table:

```
✓ Table credentials: 25 rows inserted | Pipeline: 30 raw → 2 filtered → 28 transformed → 1 deduped → 3 existing → 25 inserted (4.2s)
```

## Contoh Config Lengkap

```yaml
sheet:
  spreadsheet_id: 1ABCxyz...
  worksheet: Endpoint

table: endpoints

mapping:
  Connection:
    column: backend_id
    transform: lookup
    lookup:
      table: backends
      from: name
      to: id
  Path:
    column: path
  Method:
    column: method
  Type:
    column: type

defaults:
  id: uuid
  created_at: now
  updated_at: now
  active: true

filters:
  - column: type
    values: [http, grpc]
  - column: path
    not_empty: true
  - column: Status
    value: deleted
    not: true

unique: [path]

on_conflict:
  action: ignore
  keys: [path]
```
