package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/pterm/pterm"
	"github.com/spf13/viper"
)

var debugMode bool

// --- Logging Helpers ---

func debugLog(format string, a ...interface{}) {
	if debugMode {
		msg := fmt.Sprintf("[DEBUG] "+format, a...)
		pterm.Debug.Println(msg)
	}
}

func infoLog(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	pterm.Info.Println(msg)
}

func warnLog(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	pterm.Warning.Println(msg)
}

func errorLog(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	pterm.Error.Println(msg)
}

// --- Configuration using Viper with Environment Variables ---

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPass     string
	DBName     string
	Cooldown   int // in seconds
	RetryCount int // number of retries for API and page fetch calls
}

func loadConfig() Config {
	// Set default values
	viper.SetDefault("cooldown", 3)
	viper.SetDefault("retry_count", 3)

	// Bind environment variables (optionally with a prefix)
	viper.AutomaticEnv()
	viper.BindEnv("database.host", "DB_HOST")
	viper.BindEnv("database.port", "DB_PORT")
	viper.BindEnv("database.user", "DB_USER")
	viper.BindEnv("database.password", "DB_PASS")
	viper.BindEnv("database.name", "DB_NAME")

	// Read from config file if available
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		warnLog("Error reading config file: %v. Falling back to environment variables.", err)
	}

	return Config{
		DBHost:     viper.GetString("database.host"),
		DBPort:     viper.GetString("database.port"),
		DBUser:     viper.GetString("database.user"),
		DBPass:     viper.GetString("database.password"),
		DBName:     viper.GetString("database.name"),
		Cooldown:   viper.GetInt("cooldown"),
		RetryCount: viper.GetInt("retry_count"),
	}
}

// --- Cookie Handling ---

type Cookie struct {
	Domain         string  `json:"domain"`
	ExpirationDate float64 `json:"expirationDate"`
	HostOnly       bool    `json:"hostOnly"`
	HttpOnly       bool    `json:"httpOnly"`
	Name           string  `json:"name"`
	Path           string  `json:"path"`
	SameSite       string  `json:"sameSite"`
	Secure         bool    `json:"secure"`
	Session        bool    `json:"session"`
	StoreId        string  `json:"storeId"`
	Value          string  `json:"value"`
	ID             int     `json:"id"`
}

func loadExCookies(filePath string) (string, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	var cookies []Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return "", err
	}

	required := []string{"igneous", "ipb_pass_hash", "ipb_member_id"}
	cookieMap := make(map[string]string)
	for _, cookie := range cookies {
		for _, req := range required {
			if cookie.Name == req {
				cookieMap[req] = cookie.Value
			}
		}
	}
	for _, req := range required {
		if cookieMap[req] == "" {
			return "", fmt.Errorf("required cookie %s not found", req)
		}
	}
	cookieStr := fmt.Sprintf("igneous=%s; ipb_pass_hash=%s; ipb_member_id=%s",
		cookieMap["igneous"], cookieMap["ipb_pass_hash"], cookieMap["ipb_member_id"])
	return cookieStr, nil
}

// --- Data Structures for API and Page Entries ---

type Options struct {
	Site       string
	Offset     int64 // number of hours to offset when fetching pages
	CookieFile string
}

type PageEntry struct {
	GID    string `json:"gid"`
	Token  string `json:"token"`
	Posted string `json:"posted"`
}

type TorrentInfo struct {
	Hash  string `json:"hash"`
	Added string `json:"added"`
	Name  string `json:"name"`
	Tsize string `json:"tsize"`
	Fsize string `json:"fsize"`
}

type GalleryMetadata struct {
	Gid          int           `json:"gid"`
	Token        string        `json:"token"`
	ArchiverKey  string        `json:"archiver_key"`
	Title        string        `json:"title"`
	TitleJpn     string        `json:"title_jpn"`
	Category     string        `json:"category"`
	Thumb        string        `json:"thumb"`
	Uploader     string        `json:"uploader"`
	Posted       string        `json:"posted"`
	Filecount    string        `json:"filecount"`
	Filesize     int           `json:"filesize"`
	Expunged     bool          `json:"expunged"`
	Rating       string        `json:"rating"`
	Torrentcount string        `json:"torrentcount"`
	Torrents     []TorrentInfo `json:"torrents"`
	Tags         []string      `json:"tags"`
	ParentGid    string        `json:"parent_gid"`
	ParentKey    string        `json:"parent_key"`
	FirstGid     string        `json:"first_gid"`
	FirstKey     string        `json:"first_key"`
}

type APIResponse struct {
	Gmetadata []GalleryMetadata `json:"gmetadata"`
}

// --- Sync Structure ---

type Sync struct {
	host    string
	offset  int64
	cookies string
	db      *sql.DB
	config  Config
	client  *http.Client
}

// NewSync creates a new Sync instance based on provided options.
// It now also reads cookie data from an environment variable if not provided via a file.
func NewSync(opts Options) *Sync {
	s := &Sync{
		config: loadConfig(),
		client: &http.Client{Timeout: 15 * time.Second},
		offset: opts.Offset,
	}

	if opts.Site == "exhentai" {
		s.host = "exhentai.org"
		if opts.CookieFile == "" {
			// If cookie file is not provided, check environment variable
			if envCookie := os.Getenv("COOKIE"); envCookie != "" {
				s.cookies = envCookie
				infoLog("Using cookie from environment variable for exhentai")
			} else {
				errorLog("For exhentai, --cookie-file must be provided or COOKIE env variable must be set")
				os.Exit(1)
			}
		} else {
			c, err := loadExCookies(opts.CookieFile)
			if err != nil {
				errorLog("Error loading exhentai cookies: %v", err)
				os.Exit(1)
			}
			s.cookies = c
			infoLog("Using exhentai with provided cookie file")
		}
	} else {
		s.host = "e-hentai.org"
		// For e-hentai, check environment variable first
		if envCookie := os.Getenv("COOKIE"); envCookie != "" {
			s.cookies = envCookie
			infoLog("Using cookie from environment variable for e-hentai")
		} else {
			data, err := ioutil.ReadFile(".cookies")
			if err != nil {
				warnLog("No .cookies file found and COOKIE env variable not set, proceeding without cookies")
				s.cookies = ""
			} else {
				s.cookies = string(data)
			}
			infoLog("Using e-hentai")
		}
	}

	s.initConnection()
	return s
}

// initConnection establishes the MySQL connection.
func (s *Sync) initConnection() {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?timeout=10s",
		s.config.DBUser, s.config.DBPass, s.config.DBHost, s.config.DBPort, s.config.DBName)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		errorLog("Error opening DB: %v", err)
		os.Exit(1)
	}
	if err = db.Ping(); err != nil {
		errorLog("Error pinging DB: %v", err)
		os.Exit(1)
	}
	s.db = db
}

func (s *Sync) getLastGid() (int64, error) {
	query := "SELECT gid FROM gallery ORDER BY gid DESC LIMIT 1"
	var gid int64
	err := s.db.QueryRow(query).Scan(&gid)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return gid, nil
}

// getOffsetGid computes the starting gid based on the offset option.
// It selects the gallery entry whose posted timestamp is at least n hours older than the newest post.
func (s *Sync) getOffsetGid(offset int64) (int64, error) {
	// Get the latest posted timestamp
	var latestPosted int64
	err := s.db.QueryRow("SELECT posted FROM gallery ORDER BY posted DESC LIMIT 1").Scan(&latestPosted)
	if err != nil {
		return 0, err
	}
	threshold := latestPosted - (offset * 3600)
	var offsetGid int64
	err = s.db.QueryRow("SELECT gid FROM gallery WHERE posted <= ? ORDER BY posted DESC LIMIT 1", threshold).Scan(&offsetGid)
	if err == sql.ErrNoRows {
		return s.getLastGid()
	}
	if err != nil {
		return 0, err
	}
	return offsetGid, nil
}

// --- Page Fetching Helpers ---

func (s *Sync) getPagesByPrev(prev string) ([]PageEntry, error) {
	path := fmt.Sprintf("/?prev=%s&f_cats=0&advsearch=1&f_sname=on&f_stags=on&f_sh=&f_spf=&f_spt=&f_sfl=on&f_sfu=on", prev)
	url := "https://" + s.host + path

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*")
	req.Header.Set("Accept-Language", "en-US;q=0.9,en;q=0.8")
	req.Header.Set("DNT", "1")
	req.Header.Set("Referer", "https://"+s.host)
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.3770.142 Safari/537.36")
	if s.cookies != "" {
		req.Header.Set("Cookie", s.cookies)
	}

	var bodyStr string
	var fetchErr error
	for attempt := 0; attempt < s.config.RetryCount; attempt++ {
		resp, err := s.client.Do(req)
		if err != nil {
			fetchErr = err
			errorLog("Error fetching page on attempt %d: %v", attempt+1, err)
			time.Sleep(1 * time.Second)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			fetchErr = fmt.Errorf("HTTP status code: %d", resp.StatusCode)
			errorLog("Error fetching page on attempt %d: %v", attempt+1, fetchErr)
			time.Sleep(1 * time.Second)
			continue
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fetchErr = err
			errorLog("Error reading response on attempt %d: %v", attempt+1, err)
			time.Sleep(1 * time.Second)
			continue
		}
		bodyStr = string(body)
		break
	}
	if fetchErr != nil {
		return nil, fetchErr
	}

	if banned, waitTime := extractBanCooldown(bodyStr); banned {
		infoLog("Detected ban message. Initiating cooldown for %d seconds.", waitTime)
		runBanCooldown(waitTime)
		return s.getPagesByPrev(prev)
	}

	return parsePageEntries(bodyStr)
}

func extractBanCooldown(body string) (bool, int) {
	re := regexp.MustCompile(`(?i)The ban expires in\s*(?:(\d+)\s*days?)?\s*(?:(\d+)\s*hours?)?\s*(?:(\d+)\s*minutes?)?\s*(?:(?:and\s*)?(\d+)\s*seconds?)?`)
	matches := re.FindStringSubmatch(body)
	if len(matches) > 0 {
		days, hours, minutes, seconds := 0, 0, 0, 0
		var err error
		if matches[1] != "" {
			days, err = strconv.Atoi(matches[1])
			if err != nil {
				days = 0
			}
		}
		if matches[2] != "" {
			hours, err = strconv.Atoi(matches[2])
			if err != nil {
				hours = 0
			}
		}
		if matches[3] != "" {
			minutes, err = strconv.Atoi(matches[3])
			if err != nil {
				minutes = 0
			}
		}
		if matches[4] != "" {
			seconds, err = strconv.Atoi(matches[4])
			if err != nil {
				seconds = 0
			}
		}
		totalWait := days*86400 + hours*3600 + minutes*60 + seconds
		if totalWait > 0 {
			return true, totalWait + 10
		}
	}
	return false, 0
}

func runBanCooldown(totalWait int) {
	pb, _ := pterm.DefaultProgressbar.
		WithTotal(totalWait).
		WithTitle("Ban Cooldown").
		Start()
	for i := 0; i < totalWait; i++ {
		time.Sleep(1 * time.Second)
		pb.Add(1)
	}
	pb.Stop()
}

func parsePageEntries(body string) ([]PageEntry, error) {
	re := regexp.MustCompile(`gid=(\d+).*?t=([0-9a-f]{10}).*?>(\d{4}-\d{2}-\d{2}\s\d{2}:\d{2})<`)
	matches := re.FindAllStringSubmatch(body, -1)
	uniqueMap := make(map[string]bool)
	var list []PageEntry
	for _, m := range matches {
		if len(m) >= 4 {
			gid := m[1]
			if !uniqueMap[gid] {
				uniqueMap[gid] = true
				list = append(list, PageEntry{
					GID:    gid,
					Token:  m[2],
					Posted: m[3],
				})
			}
		}
	}
	return list, nil
}

// --- API Call ---

func (s *Sync) getMetadatas(gidlist []PageEntry) (*APIResponse, error) {
	payloadGidlist := make([][]interface{}, 0, len(gidlist))
	for _, entry := range gidlist {
		gidInt, err := strconv.Atoi(entry.GID)
		if err != nil {
			errorLog("Error converting gid %s to int: %v", entry.GID, err)
			continue
		}
		payloadGidlist = append(payloadGidlist, []interface{}{gidInt, entry.Token})
	}

	payload := map[string]interface{}{
		"method":    "gdata",
		"gidlist":   payloadGidlist,
		"namespace": 1,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := "https://api.e-hentai.org/api.php"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json;q=0.9,*/*")
	req.Header.Set("Accept-Language", "en-US;q=0.9,en;q=0.8")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DNT", "1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.3770.142 Safari/537.36")

	var apiResp *APIResponse
	for attempt := 0; attempt < s.config.RetryCount; attempt++ {
		resp, err := s.client.Do(req)
		if err != nil {
			errorLog("Error calling API on attempt %d: %v", attempt+1, err)
			time.Sleep(1 * time.Second)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			err = fmt.Errorf("HTTP status code: %d", resp.StatusCode)
			errorLog("Error calling API on attempt %d: %v", attempt+1, err)
			time.Sleep(1 * time.Second)
			continue
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			errorLog("Error reading API response on attempt %d: %v", attempt+1, err)
			time.Sleep(1 * time.Second)
			continue
		}
		var result APIResponse
		if err = json.Unmarshal(body, &result); err != nil {
			errorLog("Error unmarshalling API response on attempt %d: %v", attempt+1, err)
			time.Sleep(1 * time.Second)
			continue
		}
		apiResp = &result
		break
	}
	if apiResp == nil {
		return nil, fmt.Errorf("failed to get valid API response after %d attempts", s.config.RetryCount)
	}
	return apiResp, nil
}

// --- Database Insert Helpers ---

func (s *Sync) saveGallery(gallery GalleryMetadata) error {
	postedInt, err := strconv.ParseInt(gallery.Posted, 10, 64)
	if err != nil {
		return fmt.Errorf("parsing posted time for gid %d: %w", gallery.Gid, err)
	}
	filecountInt, _ := strconv.Atoi(gallery.Filecount)
	torrentcountInt, _ := strconv.Atoi(gallery.Torrentcount)
	rootGidInt := 0
	if gallery.ParentGid != "" {
		rootGidInt, _ = strconv.Atoi(gallery.ParentGid)
	}
	expungedInt := 0
	if gallery.Expunged {
		expungedInt = 1
	}
	stmt, err := s.db.Prepare(`
      INSERT INTO gallery (gid, token, archiver_key, title, title_jpn, category, thumb, uploader, posted, filecount, filesize, expunged, rating, torrentcount, root_gid, bytorrent)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      ON DUPLICATE KEY UPDATE token=VALUES(token), archiver_key=VALUES(archiver_key), title=VALUES(title), title_jpn=VALUES(title_jpn), category=VALUES(category), thumb=VALUES(thumb), uploader=VALUES(uploader), posted=VALUES(posted), filecount=VALUES(filecount), filesize=VALUES(filesize), expunged=VALUES(expunged), rating=VALUES(rating), torrentcount=VALUES(torrentcount), root_gid=VALUES(root_gid)
    `)
	if err != nil {
		return fmt.Errorf("preparing gallery stmt: %w", err)
	}
	defer stmt.Close()
	_, err = stmt.Exec(gallery.Gid, gallery.Token, gallery.ArchiverKey, gallery.Title, gallery.TitleJpn, gallery.Category, gallery.Thumb, gallery.Uploader, postedInt, filecountInt, gallery.Filesize, expungedInt, gallery.Rating, torrentcountInt, rootGidInt, 0)
	if err != nil {
		return fmt.Errorf("inserting gallery gid %d: %w", gallery.Gid, err)
	}
	debugLog("Inserted gallery gid %d", gallery.Gid)
	return nil
}

func (s *Sync) saveTorrent(gid int, torrent TorrentInfo, uploader string) error {
	stmt, err := s.db.Prepare(`
          INSERT INTO torrent (gid, name, hash, addedstr, fsizestr, uploader, expunged)
          VALUES (?, ?, ?, ?, ?, ?, ?)
    `)
	if err != nil {
		return fmt.Errorf("preparing torrent stmt: %w", err)
	}
	defer stmt.Close()
	_, err = stmt.Exec(gid, torrent.Name, torrent.Hash, torrent.Added, torrent.Fsize, uploader, 0)
	if err != nil {
		return fmt.Errorf("inserting torrent for gid %d: %w", gid, err)
	}
	debugLog("Inserted torrent for gid %d", gid)
	return nil
}

func (s *Sync) saveTag(gid int, tagName string) error {
	var tagID int
	err := s.db.QueryRow("SELECT id FROM tag WHERE name = ?", tagName).Scan(&tagID)
	if err == sql.ErrNoRows {
		res, err := s.db.Exec("INSERT INTO tag (name) VALUES (?)", tagName)
		if err != nil {
			if !strings.Contains(err.Error(), "Duplicate entry") {
				return fmt.Errorf("inserting tag '%s': %w", tagName, err)
			}
			err = s.db.QueryRow("SELECT id FROM tag WHERE name = ?", tagName).Scan(&tagID)
			if err != nil {
				return fmt.Errorf("querying tag '%s' after duplicate error: %w", tagName, err)
			}
		} else {
			lastID, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("getting tag id for '%s': %w", tagName, err)
			}
			tagID = int(lastID)
		}
	} else if err != nil {
		return fmt.Errorf("querying tag '%s': %w", tagName, err)
	}
	_, err = s.db.Exec("INSERT INTO gid_tid (gid, tid) VALUES (?, ?)", gid, tagID)
	if err != nil && !strings.Contains(err.Error(), "Duplicate entry") {
		return fmt.Errorf("inserting gid_tid for gid %d and tag '%s': %w", gid, tagName, err)
	}
	debugLog("Linked gallery gid %d with tag '%s'", gid, tagName)
	return nil
}

// --- Page Import and Processing ---

func (s *Sync) importPage(entries []PageEntry) (int, error) {
	pageAPIEntries := 0
	const batchSize = 25

	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]

		var apiResp *APIResponse
		var err error
		for attempt := 0; attempt < s.config.RetryCount; attempt++ {
			apiResp, err = s.getMetadatas(batch)
			if err == nil {
				break
			}
			errorLog("Error calling API for batch %d on attempt %d: %v", i/batchSize, attempt+1, err)
			time.Sleep(1 * time.Second)
		}
		if err != nil {
			errorLog("Error calling API for batch %d after %d attempts: %v", i/batchSize, s.config.RetryCount, err)
			continue
		}

		if len(apiResp.Gmetadata) == 0 {
			errorLog("API response returned no entries for batch %d", i/batchSize)
			continue
		}

		pageAPIEntries += len(apiResp.Gmetadata)

		for _, gallery := range apiResp.Gmetadata {
			if err := s.saveGallery(gallery); err != nil {
				errorLog("Error saving gallery: %v", err)
				continue
			}
			for _, t := range gallery.Torrents {
				if err := s.saveTorrent(gallery.Gid, t, gallery.Uploader); err != nil {
					errorLog("Error saving torrent for gid %d: %v", gallery.Gid, err)
				}
			}
			for _, tagName := range gallery.Tags {
				if err := s.saveTag(gallery.Gid, tagName); err != nil {
					errorLog("Error saving tag for gid %d: %v", gallery.Gid, err)
				}
			}
		}
	}
	return pageAPIEntries, nil
}

// --- Reporting ---

// generateReport queries the database to produce a final report.
// It prints the total number of entries in the gallery table,
// the last posted gallery ID, and the cutoff time formatted as "YYYY-MM-DD HH:MM UTC+0".
func (s *Sync) generateReport() error {
	var totalEntries int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM gallery").Scan(&totalEntries); err != nil {
		return fmt.Errorf("error retrieving total entry count: %w", err)
	}

	var lastGid int64
	var lastPosted int64
	if err := s.db.QueryRow("SELECT gid, posted FROM gallery ORDER BY posted DESC LIMIT 1").Scan(&lastGid, &lastPosted); err != nil {
		return fmt.Errorf("error retrieving last posted gallery: %w", err)
	}

	// Convert the posted timestamp (assumed to be Unix seconds) to a formatted UTC string.
	cutoffTime := time.Unix(lastPosted, 0).UTC().Format("2006-01-02 15:04") + " UTC+0"
	report := fmt.Sprintf("\nFINAL REPORT:\nTotal entries in database: %d\nLast posted ID: %d\nCutoff time: %s\n", totalEntries, lastGid, cutoffTime)
	infoLog(report)
	return nil
}

// run retrieves the starting gallery entry then loops fetching pages using a cooldown.
// It now uses the offset option if provided.
func (s *Sync) run() error {
	_, err := s.db.Exec("SET NAMES UTF8MB4")
	if err != nil {
		return err
	}

	var startGid int64
	if s.offset > 0 {
		startGid, err = s.getOffsetGid(s.offset)
		if err != nil {
			errorLog("Error getting offset: %v", err)
			return err
		}
		infoLog("Using offset gid: %d", startGid)
	} else {
		startGid, err = s.getLastGid()
		if err != nil {
			return err
		}
		infoLog("Got last gid = %d", startGid)
	}
	prev := strconv.FormatInt(startGid, 10)

	area, _ := pterm.DefaultArea.Start()

	for {
		time.Sleep(time.Duration(s.config.Cooldown) * time.Second)
		fetchMsg := fmt.Sprintf("Fetching page from https://%s/?prev=%s", s.host, prev)

		var pageEntries []PageEntry
		for attempt := 0; attempt < s.config.RetryCount; attempt++ {
			pageEntries, err = s.getPagesByPrev(prev)
			if err == nil {
				break
			}
			errorLog("Error fetching page on attempt %d: %v", attempt+1, err)
			time.Sleep(1 * time.Second)
		}
		if err != nil {
			errorLog("Error fetching page after %d attempts: %v", s.config.RetryCount, err)
			break
		}

		if len(pageEntries) == 0 {
			infoLog("No new entries found. Exiting loop.")
			break
		}

		pageCount := len(pageEntries)
		apiCount, err := s.importPage(pageEntries)
		if err != nil {
			errorLog("Error importing page: %v", err)
		}

		newestEntryDate := "N/A"
		if pageCount > 0 {
			t, err := time.Parse("2006-01-02 15:04", pageEntries[0].Posted)
			if err != nil {
				newestEntryDate = pageEntries[0].Posted
			} else {
				newestEntryDate = t.Format("2006-01-02")
			}
		}
		bulletItems := []pterm.BulletListItem{
			{Level: 1, Text: fmt.Sprintf("Newest Entry Date: %s", newestEntryDate)},
			{Level: 1, Text: fmt.Sprintf("Fetched Page Entries: %d", pageCount)},
			{Level: 1, Text: fmt.Sprintf("Fetched API Entries: %d", apiCount)},
		}
		bulletStr, _ := pterm.DefaultBulletList.WithItems(bulletItems).Srender()

		// Update the prev value using the newest fetched entry
		prev = pageEntries[0].GID
		nextMsg := fmt.Sprintf("Next fetch will use prev=%s", prev)
		area.Update(fetchMsg + "\n" + nextMsg + "\n" + bulletStr)
	}
	return nil
}

// --- Main ---

func main() {
	site := flag.String("site", "e-hentai", "Target site: 'e-hentai' or 'exhentai'")
	offset := flag.Int64("offset", 0, "Static offset (in hours) to adjust the initial fetch starting point. This shifts the starting entry by a fixed number of hours relative to the last processed entry.")
	cookieFile := flag.String("cookie-file", "", "Path to cookie JSON file (required for exhentai)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	debugMode = *debug

	opts := Options{
		Site:       *site,
		Offset:     *offset,
		CookieFile: *cookieFile,
	}

	instance := NewSync(opts)
	if err := instance.run(); err != nil {
		errorLog("Error: %v", err)
		os.Exit(1)
	}

	// Generate the final report after the sync loop completes.
	if err := instance.generateReport(); err != nil {
		errorLog("Error generating report: %v", err)
	}
}
