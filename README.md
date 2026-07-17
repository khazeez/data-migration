# sheet-loader

Tool migrasi data dari Google Sheets ke PostgreSQL menggunakan Go. Dikonfigurasi via YAML dan environment variable.

## Fitur

- Integrasi Google Sheets API
- Mapping kolom sheet ke kolom database (dengan rename)
- Transform value: `string_to_bool`, `lower`, `upper`, `trim`
- Filter baris berdasarkan nilai kolom tertentu
- Default value otomatis: `uuid`, `now`, `null`, `true`/`false`
- Batch insert dengan transaction dan rollback otomatis
- Deduplikasi di level aplikasi (tanpa unique index di DB)
- Conflict handling: `ignore` atau `upsert`
- Dry-run mode untuk validasi
- Semua konfigurasi via environment variable (tanpa file `app.yml`)
- Taskfile dengan auto-load `.env`

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
│   └── tables/                    # Konfigurasi per-table
├── internal/
│   ├── config/                    # Loader konfigurasi
│   ├── db/                        # Koneksi PostgreSQL & batch insert
│   ├── importer/                  # Orchestrator import
│   ├── jobs/                      # Job runner
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

## Setup

### 1. Credentials Google Sheets

Buat service account di Google Cloud Console, enable Google Sheets API, download `credentials.json`, dan letakkan di root project.

### 2. Environment Variables

Buat file `.env` di root project (lihat `.env.example`):

```env
# PostgreSQL
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=master
DB_SSLMODE=disable

# Google Sheets
GOOGLE_CREDENTIALS=credentials.json

# App
JOBS_DIR=configs/jobs
TABLES_DIR=configs/tables
LOG_LEVEL=info
```

Atau bisa juga set via environment variable sistem langsung.

### 3. Konfigurasi Table

Buat file YAML di `configs/tables/`. Contoh: `customer.yml`

```yaml
sheet:
  spreadsheet_id: 1ABCxyz...              # ID Google Sheet
  worksheet: migrasi test                  # Nama worksheet/tab

table: customer                            # Nama tabel di PostgreSQL

mapping:
  nama:                                    # Kolom di sheet
    column: name                           # Kolom di database
  umur:
    column: age
  script:
    column: script
  status:                                  # Kolom sheet "status"
    column: active                         # Mapping ke kolom DB "active"
    transform: string_to_bool              # Transform: "active"/"inactive" → true/false

filter:                                    # Hanya insert baris tertentu
  column: status                           # Kolom sheet (bukan kolom DB)
  value: active                            # Hanya baris dengan value "active"

defaults:
  id: uuid                                 # Auto-generate UUID
  timestamp: now                           # Timestamp current time

on_conflict:                               # Conflict handling
  action: ignore                           # ignore atau upsert
  keys: [name]                             # Kolom untuk identifikasi duplicate
```

### 4. Konfigurasi Job

Buat file YAML di `configs/jobs/`. Contoh: `master-data.yml`

```yaml
name: master-data                          # Nama job (gunakan di -job flag)
description: Master data migration         # Deskripsi opsional
tables:
  - customer                               # Nama file table (tanpa .yml)
```

## Tutorial Lengkap

### Step 1: Persiapan Environment

1. Copy `.env.example` ke `.env` dan isi konfigurasi PostgreSQL serta path credentials.
2. Pastikan `credentials.json` dari Google Cloud Console sudah ada di root project.
3. Buat tabel tujuan di PostgreSQL. Contoh:

```sql
CREATE TABLE customer (
    id UUID PRIMARY KEY,
    name VARCHAR(255),
    age INTEGER,
    script TEXT,
    active BOOLEAN DEFAULT true,
    timestamp TIMESTAMPTZ DEFAULT NOW()
);
```

### Step 2: Buat Table Config

Buat file `configs/tables/customer.yml`:

```yaml
sheet:
  spreadsheet_id: 11xTqz5xSSFT_9FzgoO3iwn0U-TcKaezTjm62cRq3WQI
  worksheet: migrasi test

table: customer

mapping:
  nama:
    column: name
  umur:
    column: age
  script:
    column: script
  status:
    column: active
    transform: string_to_bool

filter:
  column: status
  value: active

defaults:
  id: uuid
  timestamp: now

on_conflict:
  action: ignore
  keys: [name]
```

Penjelasan:
- **mapping**: Kolom `status` di sheet di-rename jadi `active` di DB, dan di-transform dari string ("active"/"inactive") ke boolean.
- **filter**: Hanya baris dengan `status = "active"` yang akan di-insert. Baris dengan status lain akan difilter.
- **defaults**: Kolom `id` diisi UUID otomatis, `timestamp` diisi waktu sekarang.
- **on_conflict**: Jika ada duplikat berdasarkan `name`, skip (ignore).

### Step 3: Buat Job Config

Buat file `configs/jobs/master-data.yml`:

```yaml
name: master-data
description: Migrasi master data customer
tables:
  - customer
```

### Step 4: Dry-Run

Jalankan dry-run untuk validasi tanpa insert ke database:

```bash
go run ./cmd/ -job master-data -dry-run
```

atau via Taskfile:

```bash
task run -- -dry-run
```

Output akan menunjukkan:
- Jumlah baris yang dibaca dari sheet
- Jumlah baris yang difilter
- Jumlah baris yang akan di-insert
- Sample baris pertama
- Konfigurasi filter dan conflict

### Step 5: Jalankan Migrasi

```bash
go run ./cmd/ -job master-data
```

atau via Taskfile:

```bash
task run
```

## Transform Values

Transform diterapkan pada value kolom setelah dibaca dari sheet, sebelum di-insert ke database.

| Transform | Input Contoh               | Output  |
|-----------|----------------------------|---------|
| `string_to_bool` | "Y", "yes", "1", "true", "t", "active", "deployed" | `true` |
| `string_to_bool` | "N", "no", "0", "false", "f", "inactive", "undeployed" | `false` |
| `lower`   | "Hello World"              | "hello world" |
| `upper`   | "Hello World"              | "HELLO WORLD" |
| `trim`    | "  value  "               | "value" |

Contoh mapping dengan transform:

```yaml
mapping:
  status:
    column: is_active
    transform: string_to_bool
  nama:
    column: name_lower
    transform: lower
```

## Filter

Filter bekerja pada **sheet column names** (sebelum transform/mapping), jadi nilai yang dibandingkan adalah nilai mentah dari sheet.

```yaml
filter:
  column: status        # Nama kolom di sheet
  value: active         # Value yang dicocokkan (string)
```

Hanya baris dengan nilai sesuai `value` yang akan diproses lebih lanjut.

## Conflict Handling

Dua mode conflict handling:

### ignore (default)

```yaml
on_conflict:
  action: ignore
  keys: [name]
```

Aplikasi akan query data existing terlebih dahulu, lalu skip baris yang sudah ada. Tidak memerlukan unique index di PostgreSQL.

### upsert

```yaml
on_conflict:
  action: upsert
  keys: [name]
```

Menggunakan `ON CONFLICT ... DO UPDATE` di PostgreSQL. **Membutuhkan unique index** di tabel.

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-job` | (required) | Nama job dari `configs/jobs/` |
| `-dry-run` | `false` | Validasi tanpa insert |
| `-batch` | `500` | Ukuran batch insert |
| `-log-level` | `info` | Level log: `debug`, `info`, `warn`, `error` |

## Taskfile Commands

```bash
task run          # Jalankan migrasi (default job: master-data)
task dry-run      # Dry-run mode
task list         # List available jobs
task build        # Build binary
```

Gunakan `--` untuk oper ke flag tambahan:

```bash
task run -- -job master-data -batch 200 -log-level debug
task dry-run -- -job master-data
task list
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DB_HOST` | ✓ | - | Host PostgreSQL |
| `DB_PORT` | ✓ | `5432` | Port PostgreSQL |
| `DB_USER` | ✓ | - | User PostgreSQL |
| `DB_PASSWORD` | ✓ | - | Password PostgreSQL |
| `DB_NAME` | ✓ | - | Nama database |
| `DB_SSLMODE` | - | `disable` | SSL mode |
| `GOOGLE_CREDENTIALS` | ✓ | - | Path ke credentials.json |
| `JOBS_DIR` | - | `configs/jobs` | Directory job config |
| `TABLES_DIR` | - | `configs/tables` | Directory table config |
| `LOG_LEVEL` | - | `info` | Level log |

Semua environment variable otomatis di-load dari file `.env` oleh Taskfile.

## Alur Migrasi

1. Baca konfigurasi table dari YAML
2. Baca data dari Google Sheets API
3. Validasi header sheet vs mapping
4. **Filter** baris berdasarkan kondisi filter
5. **Transform** value (rename kolom, string_to_bool, dll)
6. Apply **default values** untuk kolom yang tidak ada di sheet
7. **Deduplikasi** (jika on_conflict = ignore): query existing data, skip duplikat
8. **Batch insert** dalam transaction
9. Commit atau rollback jika error
