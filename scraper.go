package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"4d63.com/homedir"
	"cloud.google.com/go/storage"
	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
)

// Config holds external parameters for the scraper.
type Config struct {
	SearchQuery    string `json:"search_query"`
	SearchService  string `json:"search_service"`
	ResultType     string `json:"result_type"`
	OutputFilePath string `json:"output_file_path"`
	UseGCS bool `json:"useGCS"`
	GCSBucket string `json:"GCSBucket"`
}

// RSS defines the root element of the RSS feed.
type RSS struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel Channel  `xml:"channel"`
}

// Channel defines the core information block for the feed.
type Channel struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Language    string `xml:"language"`
	Items       []Item `xml:"item"`
}

// Item defines a single article entry in the feed.
type Item struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	PubDate string `xml:"pubDate"` // RFC1123Z
	Guid    string `xml:"guid"`
	ParsedDate time.Time `xml:"-"`
}

// CSS Selectors
const articleContainerSelector = "div.elBtDR div[class^='ArticleResults__SearchItemContainer']"
const articleLinkSelector = "a"
const titleSelector = "h3"
const dateContainerSelector = "div[class*='ArticleResults__DetailsLine']"

const baseURL = "https://yle.fi"

// Global variables initialized in init()
var (
	absPath       string
	currentConfig *Config
)

func init() {
	var err error
	currentConfig, err = loadConfig("config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	absPath, err = homedir.Expand(currentConfig.OutputFilePath)
	if err != nil {
		log.Fatalf("Failed to expand path: %v", err)
	}
	
	svcAccPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if svcAccPath == "" && currentConfig.UseGCS {
		log.Fatalf("Opted for GCP but not specified credentials.")
	}
	
}

func main() {
	log.Printf("GCS is set to %t", currentConfig.UseGCS)
	var allItems []Item
	page := 1

	for {
		targetURL := buildURL(currentConfig, page)
		log.Printf("Scraping page %d: %s", page, targetURL)

		pageItems, err := fetchAndParsePage(targetURL)
		if err != nil {
			// A timeout or error here usually indicates the end of pagination (no results container found)
			log.Printf("Page %d scrape ended (likely no more results): %v", page, err)
			break
		}

		if len(pageItems) == 0 {
			if page == 1 {
				log.Println("Warning: No articles found on first page.")
			} else {
				log.Printf("Finished scraping. Page %d returned no valid articles.", page)
			}
			break
		}

		log.Printf("Found %d articles on page %d.", len(pageItems), page)
		allItems = append(allItems, pageItems...)
		page++

		time.Sleep(2 * time.Second)
	}
	
	// Sort articles by ParsedDate, newest first (Descending order)
	if len(allItems) > 0 {
		sort.Slice(allItems, func(i, j int) bool {
			// This ensures newest dates (greater time values) come first.
			// Articles with a zero-value time (failed parse) will be pushed to the end.
			return allItems[i].ParsedDate.After(allItems[j].ParsedDate) 
		})
		log.Println("Results sorted by date (newest to oldest).")
	}

	if len(allItems) == 0 {
		log.Println("No articles found.")
	} else {
		log.Printf("Total articles: %d", len(allItems))
	}

	baseSearchURL := buildURL(currentConfig, 1)
	rssFeed := generateRSS(allItems, baseSearchURL, currentConfig.SearchQuery)

	xmlOutput, err := xml.MarshalIndent(rssFeed, "", "  ")
	if err != nil {
		log.Fatalf("Error marshaling XML: %v", err)
	}

	finalOutput := []byte(xml.Header + string(xmlOutput))

	if err := writeRSSFile(absPath, finalOutput); err != nil {
		log.Fatalf("Error writing file: %v", err)
	}

	log.Printf("RSS feed saved to: %s", absPath)
		
	shouldUpload := currentConfig.UseGCS
	var uploadName string = fmt.Sprintf("%s.xml", strings.ReplaceAll(currentConfig.SearchQuery, " ", ""))
	if shouldUpload {
		log.Printf("GCS option is ON.")
		if err := uploadRSStoGCS(absPath, currentConfig.GCSBucket, uploadName); err != nil {
			log.Fatalf("Fatal: %v", err)
		}
		log.Printf("Writing to bucket: %s with name %s.", currentConfig.GCSBucket, uploadName)
		
	} else {
		log.Printf("GCS option is OFF. Not uploading to GCS. Local file is at %s", absPath)
	}
}

func fetchAndParsePage(url string) ([]Item, error) {
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 10*time.Second)
	defer timeoutCancel()

	var finalHTML string
	err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(articleContainerSelector, chromedp.ByQuery),
		chromedp.OuterHTML("html", &finalHTML),
	)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(finalHTML))
	if err != nil {
		return nil, fmt.Errorf("HTML parse error: %w", err)
	}

	var items []Item
	doc.Find(articleContainerSelector).Each(func(i int, s *goquery.Selection) {
		linkSelection := s.Find(articleLinkSelector).First()
		href, exists := linkSelection.Attr("href")

		if !exists {
			return
		}

		if !strings.HasPrefix(href, "http") {
			href = baseURL + href
		}

		title := strings.TrimSpace(linkSelection.Find(titleSelector).Text())
		if title == "" {
			return
		}

		dateString := s.Find(dateContainerSelector).First().Text()
		if index := strings.Index(dateString, "|"); index != -1 {
			dateString = dateString[:index]
		}
		dateString = strings.TrimSpace(dateString)

		// Set ParsedDate to the zero-value time (the oldest possible time) by default.
		parsedDate := time.Time{}
		// Set PubDate to current time as a last resort fallback for RSS spec compliance.
		pubDate := time.Now().Format(time.RFC1123Z)
		
		if t, err := parseFinnishDate(dateString); err == nil {
			parsedDate = t
			pubDate = t.Format(time.RFC1123Z)
		}

		items = append(items, Item{
			Title:      title,
			Link:       href,
			PubDate:    pubDate,
			Guid:       href,
			ParsedDate: parsedDate, 
		})
	})

	return items, nil
}

func writeRSSFile(filePath string, data []byte) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir failed: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	return nil
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file error: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("json parse error: %w", err)
	}

	if cfg.SearchQuery == "" || cfg.SearchService == "" || cfg.OutputFilePath == "" {
		return nil, fmt.Errorf("missing config parameters")
	}

	return &cfg, nil
}

func buildURL(cfg *Config, page int) string {
	// Quote the query to force exact match
	exactQuery := fmt.Sprintf(`"%s"`, cfg.SearchQuery)
	q := url.QueryEscape(exactQuery)

	return fmt.Sprintf("https://haku.yle.fi/?page=%d&query=%s&service=%s&type=%s",
		page, q, cfg.SearchService, cfg.ResultType)
}

func parseFinnishDate(dateStr string) (time.Time, error) {
	dateStr = strings.TrimSpace(dateStr)
	loc, _ := time.LoadLocation("Europe/Helsinki")

	// Try DD.MM.YYYY
	if t, err := time.ParseInLocation("2.1.2006", dateStr, loc); err == nil {
		return t, nil
	}

	// Try DD.MM. (assume current year)
	if strings.Contains(dateStr, ".") && !strings.Contains(dateStr, time.Now().Format("2006")) {
		currentYear := time.Now().Year()
		dateWithYear := fmt.Sprintf("%s%d", dateStr, currentYear)
		if t, err := time.ParseInLocation("2.1.2006", dateWithYear, loc); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported date format: %s", dateStr)
}

func generateRSS(items []Item, url, query string) RSS {
	title := fmt.Sprintf("Yle Search Results for '%s'", query)
	description := fmt.Sprintf("Articles from Yle generated via scraping for: %s", query)

	return RSS{
		Version: "2.0",
		Channel: Channel{
			Title:       title,
			Link:        url,
			Description: description,
			Language:    "fi",
			Items:       items,
		},
	}
}

func uploadRSStoGCS(localFilePath, bucketName, objectName string) error {
	ctx := context.Background()
	
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	defer client.Close()
	
	f, err := os.Open(localFilePath)
	if err != nil { 
		return fmt.Errorf("%w", err)
	}
	defer f.Close()
	
	bucket := client.Bucket(bucketName)
	obj := bucket.Object(objectName)
	
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	
	wc := obj.NewWriter(ctx)
	
	wc.ContentType = "application/xml"
	wc.Metadata = map[string]string{
		"created-by": "yle-scraper",
		"date-run": time.Now().Format(time.RFC3339),
	}
	
	if _, err = io.Copy(wc, f); err != nil {
		return fmt.Errorf("%w", err)
	}
	
	if err := wc.Close(); err != nil {
		return fmt.Errorf("%w", err)
	}
	
	log.Printf("File %s successfully uploaded to gs://%s/%s", localFilePath, bucketName, objectName)
	return nil
}