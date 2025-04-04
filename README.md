# e-hentai-db-go

A command-line tool written in Go that synchronizes gallery data from e-hentai/exhentai websites into a MySQL database. The tool fetches page entries via HTTP, calls the e-hentai API for metadata, and then inserts or updates the database. At the end of the sync process, it generates a report with total entry count, the last posted gallery ID, and the cutoff time in UTC.

## Database dump
- [Releases](https://github.com/TAY0123/e-hentai-db-go/releases)

## Requirements

- Go (version 1.14+ recommended)
- MySQL Database
- [Viper](https://github.com/spf13/viper)
- [pterm](https://github.com/pterm/pterm)
- [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql)

## Build

1. **Clone the Repository:**

   ```bash
   git clone https://github.com/TAY0123/e-hentai-db-go.git
   cd e-hentai-db-go
   ```

2. **Install Dependencies:**

   ```bash
   go get
   ```

3. **Build the Binary:**

   ```bash
   go build
   ```

## Configuration

Create a `config.yaml` file in the root directory with the following structure:

```yaml
database:
  host: "127.0.0.1"
  port: "3306"
  user: "your_db_user"
  password: "your_db_password"
  name: "your_db_name"
sleep_duration: 10 #recommanded
retry_count: 3
```

Alternatively, you can override these settings using environment variables:

- `DB_HOST`
- `DB_PORT`
- `DB_USER`
- `DB_PASS`
- `DB_NAME`
- `COOKIE`
- `SLEEP_DURATION`

## Usage
If you want to parse exhentai remember to export cookie json from the browser and save to cookie.json file

Run the sync tool with the following options:

```bash
./e-hentai-sync --site="exhentai" --offset=24 --cookie-file="path/to/cookie.json"
```

### Command-Line Flags

- **`--site`**:  
  Target site; use either `"e-hentai"` or `"exhentai"`.

- **`--offset`**:  
  Offset (in hours) to adjust the initial fetch starting point. This shifts the starting entry by a fixed number of hours relative to the last processed entry.

- **`--cookie-file`**:  
  Path to a cookie JSON file (required for exhentai). If not provided, the tool will look for the `COOKIE` environment variable.

- **`--debug`**:  
  Enable debug logging.

- **`--only-expunged`**:  
  Fetch only expunged entry.

- **`--also-expunged`**:  
  Also fetch expunged entry after normal entry updated.

- **`--search`**:  
  search query for filter result: [Gallery Searching](https://ehwiki.org/wiki/Gallery_Searching)

## Contributing

Contributions are welcome! Please open issues or submit pull requests with improvements, bug fixes, or new features.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.