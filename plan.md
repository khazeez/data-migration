Dari desain yang kita sepakati, tool ini sudah masuk kategori aplikasi yang cukup besar. Kalau dibuat lengkap, isinya akan mencakup sekitar:

25–40 file Go
±2.500–4.000 baris kode
Integrasi Google Sheets API
PostgreSQL (pgx)
YAML parser
UUID generator
Batch insert
Job runner
Logger
Error handling
Transaction
CLI
README
Contoh konfigurasi

Itu jauh melebihi batas panjang satu respons ChatGPT, jadi aku tidak bisa menghasilkan semuanya sekaligus tanpa terpotong.

Yang aku usulkan

Kita bangun seperti proyek open-source secara bertahap sampai selesai. Aku akan memberikan setiap file lengkap, bukan potongan kode atau pseudo-code.

Target akhirnya:

sheet-loader/
│
├── cmd/
│   └── main.go
│
├── configs/
│   ├── app.yml
│   ├── jobs/
│   │   └── master-data.yml
│   └── tables/
│       ├── role.yml
│       ├── vendor.yml
│       └── ...
│
├── internal/
│   ├── config/
│   ├── db/
│   ├── sheets/
│   ├── mapper/
│   ├── importer/
│   ├── jobs/
│   ├── logger/
│   ├── validator/
│   ├── transform/
│   └── utils/
│
├── credentials.json
├── go.mod
├── README.md
└── Makefile

Fitur yang akan kita buat:

✅ Google Sheets API
✅ YAML config
✅ Job runner
✅ UUID otomatis
✅ Default value (now, uuid, null, dll.)
✅ Batch insert PostgreSQL
✅ Transaction
✅ Rollback
✅ Dry-run
✅ Logging
✅ Validasi header
✅ Auto mapping
✅ Progress output
✅ Error report