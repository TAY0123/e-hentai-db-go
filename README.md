# e-hentai-db-go

A command-line tool written in Go that synchronizes gallery data from e-hentai/exhentai websites into a MySQL database. The tool fetches page entries via HTTP, calls the e-hentai API for metadata, and then inserts or updates the database. At the end of the sync process, it generates a report with total entry count, the last posted gallery ID, and the cutoff time in UTC.

## Requirements

- Go (version 1.14+ recommended)
- MySQL Database
- [Viper](https://github.com/spf13/viper) for configuration management
- [pterm](https://github.com/pterm/pterm) for terminal UI enhancements
- [go-sql-driver/mysql](https://github.com/go-sql-driver/mysql) for MySQL connectivity

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
cooldown: 10 #recommanded
retry_count: 3
```

Alternatively, you can override these settings using environment variables:

- `DB_HOST`
- `DB_PORT`
- `DB_USER`
- `DB_PASS`
- `DB_NAME`
- `COOKIE`

## Usage

Run the sync tool with the following options:

```bash
./sync --site="exhentai" --overlap=24 --cookie-file="path/to/cookie.json" --debug
```

### Command-Line Flags

- **`--site`**:  
  Target site; use either `"e-hentai"` or `"exhentai"`.

- **`--offset`**:  
  Static offset (in hours) to adjust the initial fetch starting point. This shifts the starting entry by a fixed number of hours relative to the last processed entry.

- **`--cookie-file`**:  
  Path to a cookie JSON file (required for exhentai). If not provided, the tool will look for the `COOKIE` environment variable.

- **`--debug`**:  
  Enable debug logging.

## Contributing

Contributions are welcome! Please open issues or submit pull requests with improvements, bug fixes, or new features.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

---

This README provides a clear overview of the project, setup instructions, usage examples, and detailed descriptions of the offset and overlap options to help users understand the tool’s functionality.
