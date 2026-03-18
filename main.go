package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Colors for terminal output
type Colors struct {
	GREEN    string
	RED      string
	BLUE     string
	SKY_BLUE string
	YELLOW   string
	CYAN     string
	MAGENTA  string
	BOLD     string
	END      string
}

var color = Colors{
	GREEN:    "\033[32m",
	RED:      "\033[31m",
	BLUE:     "\033[34m",
	SKY_BLUE: "\033[38;5;153m",
	YELLOW:   "\033[33m",
	CYAN:     "\033[36m",
	MAGENTA:  "\033[35m",
	BOLD:     "\033[1m",
	END:      "\033[0m",
}

var log = logrus.New()

// CrawlResult represents the result of crawling a single URL
type CrawlResult struct {
	URL     string   `json:"url"`
	Status  int      `json:"status"`
	Buckets []string `json:"buckets"`
	Error   string   `json:"error,omitempty"`
}

// OutputFiles handles file output operations
type OutputFiles struct {
	outputFile  string
	bucketsFile string
	mu          sync.Mutex
}

// NewOutputFiles creates a new OutputFiles instance
func NewOutputFiles(outputFile string) *OutputFiles {
	var bucketsFile string
	if outputFile != "" {
		ext := ""
		idx := strings.LastIndex(outputFile, ".")
		if idx != -1 {
			ext = outputFile[idx:]
			outputFile = outputFile[:idx]
		}
		bucketsFile = outputFile + "_buckets.txt"
		outputFile = outputFile + ext
	}

	return &OutputFiles{
		outputFile:  outputFile,
		bucketsFile: bucketsFile,
	}
}

// MakeFile creates the output files
func (of *OutputFiles) MakeFile() error {
	// Create main output file
	if _, err := os.Stat(of.outputFile); err == nil {
		log.Infof("%s[+] File already exists: %s%s", color.GREEN, of.outputFile, color.END)
	} else {
		if _, err := os.Create(of.outputFile); err != nil {
			return fmt.Errorf("error creating file: %w", err)
		}
		log.Infof("%s[+] File created: %s%s", color.GREEN, of.outputFile, color.END)
	}

	// Create buckets file
	if of.bucketsFile != "" {
		f, err := os.Create(of.bucketsFile)
		if err != nil {
			return fmt.Errorf("error creating buckets file: %w", err)
		}
		defer f.Close()

		f.WriteString("# AWS S3 Buckets Found\n")
		f.WriteString("# Format: bucket-name | source-url\n")
		f.WriteString("# " + strings.Repeat("=", 76) + "\n\n")
		log.Infof("%s[+] Buckets file created: %s%s", color.GREEN, of.bucketsFile, color.END)
	}

	return nil
}

// SaveBucketImmediately saves a bucket to file immediately when found (thread-safe)
func (of *OutputFiles) SaveBucketImmediately(bucket, sourceURL string) {
	if of.bucketsFile == "" {
		return
	}

	of.mu.Lock()
	defer of.mu.Unlock()

	f, err := os.OpenFile(of.bucketsFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Errorf("%s(-) Error saving bucket: %v%s", color.RED, err, color.END)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "%s | %s\n", bucket, sourceURL)

	// Print to terminal with highlighting
	fmt.Printf("\n%s%s%s%s\n", color.BOLD, color.GREEN, strings.Repeat("─", 80), color.END)
	fmt.Printf("%s%s[BUCKET DISCOVERED]%s\n", color.BOLD, color.GREEN, color.END)
	fmt.Printf("%s  Name:   %s%s\n", color.GREEN, bucket, color.END)
	fmt.Printf("%s  Source: %s%s\n", color.CYAN, sourceURL, color.END)
	fmt.Printf("%s%s%s%s\n\n", color.BOLD, color.GREEN, strings.Repeat("─", 80), color.END)
}

// SaveResults saves crawling results to file
func (of *OutputFiles) SaveResults(results []CrawlResult, format string) error {
	switch format {
	case "json":
		return of.saveJSON(results)
	case "csv":
		return of.saveCSV(results)
	default:
		return of.saveTXT(results)
	}
}

func (of *OutputFiles) saveJSON(results []CrawlResult) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(of.outputFile, data, 0644)
}

func (of *OutputFiles) saveCSV(results []CrawlResult) error {
	f, err := os.Create(of.outputFile)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// Write header
	writer.Write([]string{"URL", "Status", "Buckets Found", "Bucket Names", "Error"})

	// Write rows
	for _, r := range results {
		status := ""
		if r.Status != 0 {
			status = fmt.Sprintf("%d", r.Status)
		}

		writer.Write([]string{
			r.URL,
			status,
			fmt.Sprintf("%d", len(r.Buckets)),
			strings.Join(r.Buckets, ", "),
			r.Error,
		})
	}

	return nil
}

func (of *OutputFiles) saveTXT(results []CrawlResult) error {
	f, err := os.Create(of.outputFile)
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString("AWS S3 Bucket Crawler Results\n")
	f.WriteString(strings.Repeat("=", 80) + "\n\n")

	for _, r := range results {
		status := r.Error
		if r.Status != 0 {
			status = fmt.Sprintf("%d", r.Status)
		}

		fmt.Fprintf(f, "URL: %s\n", r.URL)
		fmt.Fprintf(f, "Status: %s\n", status)

		if len(r.Buckets) > 0 {
			fmt.Fprintf(f, "Buckets found (%d):\n", len(r.Buckets))
			for _, bucket := range r.Buckets {
				fmt.Fprintf(f, "  - %s\n", bucket)
			}
		} else {
			f.WriteString("No buckets found\n")
		}
		f.WriteString(strings.Repeat("-", 80) + "\n\n")
	}

	return nil
}

// S3PatternMatcher handles S3 bucket pattern matching and validation
type S3PatternMatcher struct {
	patterns      []*regexp.Regexp
	outputHandler *OutputFiles
}

// NewS3PatternMatcher creates a new S3PatternMatcher
func NewS3PatternMatcher(outputHandler *OutputFiles) *S3PatternMatcher {
	patterns := []string{
		// https://s3.amazonaws.com/bucket-name/
		`(?i)https?://s3\.amazonaws\.com/([a-z0-9][a-z0-9\-\.]{1,61}[a-z0-9])/?`,

		// https://s3-region.amazonaws.com/bucket-name/
		`(?i)https?://s3-([a-z0-9\-]+)\.amazonaws\.com/([a-z0-9][a-z0-9\-\.]{1,61}[a-z0-9])/?`,

		// https://bucket-name.s3.amazonaws.com/
		`(?i)https?://([a-z0-9][a-z0-9\-\.]{1,61}[a-z0-9])\.s3\.amazonaws\.com/?`,

		// https://bucket-name.s3-region.amazonaws.com/
		`(?i)https?://([a-z0-9][a-z0-9\-\.]{1,61}[a-z0-9])\.s3-([a-z0-9\-]+)\.amazonaws\.com/?`,

		// https://bucket-name.s3.region.amazonaws.com/
		`(?i)https?://([a-z0-9][a-z0-9\-\.]{1,61}[a-z0-9])\.s3\.([a-z0-9\-]+)\.amazonaws\.com/?`,

		// s3://bucket-name/
		`(?i)s3://([a-z0-9][a-z0-9\-\.]{1,61}[a-z0-9])/?`,

		// arn:aws:s3:::bucket-name
		`(?i)arn:aws:s3:::([a-z0-9][a-z0-9\-\.]{1,61}[a-z0-9])`,

		// Bucket name in quotes or config
		`(?i)"bucket"\s*:\s*"([a-z0-9][a-z0-9\-\.]{1,61}[a-z0-9])"`,
		`(?i)'bucket'\s*:\s*'([a-z0-9][a-z0-9\-\.]{1,61}[a-z0-9])'`,

		// AWS SDK configurations
		`(?i)Bucket\s*[:=]\s*["']([a-z0-9][a-z0-9\-\.]{1,61}[a-z0-9])["']`,

		// In JavaScript/JSON context
		`(?i)bucketName\s*[:=]\s*["']([a-z0-9][a-z0-9\-\.]{1,61}[a-z0-9])["']`,
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		compiled = append(compiled, regexp.MustCompile(p))
	}

	log.Debugf("%s[*] Compiled %d S3 bucket regex patterns%s", color.CYAN, len(compiled), color.END)

	return &S3PatternMatcher{
		patterns:      compiled,
		outputHandler: outputHandler,
	}
}

// ExtractBuckets extracts AWS S3 bucket names from content
func (spm *S3PatternMatcher) ExtractBuckets(content, sourceURL string) []string {
	bucketsMap := make(map[string]bool)
	log.Debugf("%s[*] Extracting buckets from %d characters of content%s", color.CYAN, len(content), color.END)

	for idx, pattern := range spm.patterns {
		matches := pattern.FindAllStringSubmatch(content, -1)
		if len(matches) > 0 {
			log.Debugf("%s[*] Pattern %d found %d potential match(es)%s", color.CYAN, idx+1, len(matches), color.END)
		}

		for _, match := range matches {
			// Get the last captured group (bucket name)
			var bucket string
			for i := len(match) - 1; i >= 0; i-- {
				if match[i] != "" && i != 0 {
					bucket = match[i]
					break
				}
			}

			if bucket != "" && spm.isValidBucketName(bucket) {
				bucketLower := strings.ToLower(bucket)
				if !bucketsMap[bucketLower] {
					log.Debugf("%s[+] Valid bucket found: %s%s", color.GREEN, bucketLower, color.END)

					// Save immediately to file and print to terminal
					if spm.outputHandler != nil {
						spm.outputHandler.SaveBucketImmediately(bucketLower, sourceURL)
					}
				}
				bucketsMap[bucketLower] = true
			}
		}
	}

	// Convert map to slice
	buckets := make([]string, 0, len(bucketsMap))
	for bucket := range bucketsMap {
		buckets = append(buckets, bucket)
	}

	if len(buckets) > 0 {
		log.Infof("%s[+] Extracted %d unique bucket(s)%s", color.GREEN, len(buckets), color.END)
	} else {
		log.Debugf("%s[!] No buckets found in content%s", color.YELLOW, color.END)
	}

	return buckets
}

// isValidBucketName validates if the string is a valid S3 bucket name
func (spm *S3PatternMatcher) isValidBucketName(name string) bool {
	if len(name) < 3 || len(name) > 63 {
		return false
	}

	// Must start and end with lowercase letter or number
	if !regexp.MustCompile(`^[a-z0-9].*[a-z0-9]$`).MatchString(strings.ToLower(name)) {
		return false
	}

	// Can only contain lowercase letters, numbers, dots, and hyphens
	if !regexp.MustCompile(`^[a-z0-9\-\.]+$`).MatchString(strings.ToLower(name)) {
		return false
	}

	// Cannot be formatted as an IP address
	if regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`).MatchString(name) {
		return false
	}

	// Avoid common false positives
	falsePositives := []string{"www", "api", "cdn", "static", "assets", "example", "test"}
	nameLower := strings.ToLower(name)
	for _, fp := range falsePositives {
		if nameLower == fp {
			return false
		}
	}

	return true
}

// WebCrawler handles web crawling and bucket extraction
type WebCrawler struct {
	timeout        time.Duration
	maxWorkers     int
	userAgent      string
	patternMatcher *S3PatternMatcher
	client         *http.Client
}

// NewWebCrawler creates a new WebCrawler
func NewWebCrawler(timeout int, maxWorkers int, userAgent string, outputHandler *OutputFiles) *WebCrawler {
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	}

	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	log.Infof("%s[+] Crawler initialized (timeout=%ds, workers=%d)%s", color.GREEN, timeout, maxWorkers, color.END)

	return &WebCrawler{
		timeout:        time.Duration(timeout) * time.Second,
		maxWorkers:     maxWorkers,
		userAgent:      userAgent,
		patternMatcher: NewS3PatternMatcher(outputHandler),
		client:         client,
	}
}

// CrawlURL crawls a single URL and extracts S3 buckets
func (wc *WebCrawler) CrawlURL(url string) CrawlResult {
	result := CrawlResult{
		URL:     url,
		Buckets: []string{},
	}

	log.Infof("%s[*] Crawling: %s%s", color.BLUE, url, color.END)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		result.Error = fmt.Sprintf("Request error: %v", err)
		log.Errorf("%s(-) Request error for %s: %v%s", color.RED, url, err, color.END)
		return result
	}

	req.Header.Set("User-Agent", wc.userAgent)

	resp, err := wc.client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			result.Error = "Timeout"
			log.Errorf("%s(-) Timeout while crawling %s%s", color.RED, url, color.END)
		} else if strings.Contains(err.Error(), "too many redirects") {
			result.Error = "Too many redirects"
			log.Errorf("%s(-) Too many redirects for %s%s", color.RED, url, color.END)
		} else {
			result.Error = fmt.Sprintf("Request error: %v", err)
			log.Errorf("%s(-) Request error for %s: %v%s", color.RED, url, err, color.END)
		}
		return result
	}
	defer resp.Body.Close()

	result.Status = resp.StatusCode
	log.Debugf("%s[*] Response: HTTP %d%s", color.CYAN, resp.StatusCode, color.END)

	if resp.StatusCode == 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			result.Error = fmt.Sprintf("Read error: %v", err)
			log.Errorf("%s(-) Read error for %s: %v%s", color.RED, url, err, color.END)
			return result
		}

		content := string(body)
		log.Debugf("%s[*] Content retrieved: %d characters%s", color.CYAN, len(content), color.END)

		// Extract buckets from content
		result.Buckets = wc.patternMatcher.ExtractBuckets(content, url)

		if len(result.Buckets) > 0 {
			log.Infof("%s[+] Found %d bucket(s) in %s%s", color.GREEN, len(result.Buckets), url, color.END)
		} else {
			log.Infof("%s[!] No buckets found in %s%s", color.YELLOW, url, color.END)
		}
	} else {
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		log.Warnf("%s[!] Non-200 status: HTTP %d for %s%s", color.YELLOW, resp.StatusCode, url, color.END)
	}

	return result
}

// CrawlURLs crawls multiple URLs concurrently
func (wc *WebCrawler) CrawlURLs(urls []string, verbose bool) []CrawlResult {
	results := make([]CrawlResult, 0, len(urls))
	var resultsMu sync.Mutex

	log.Infof("%s[+] Starting concurrent crawl of %d URL(s)%s", color.GREEN, len(urls), color.END)

	// Create worker pool
	jobs := make(chan string, len(urls))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < wc.maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for url := range jobs {
				result := wc.CrawlURL(url)

				resultsMu.Lock()
				results = append(results, result)
				count := len(results)
				resultsMu.Unlock()

				if verbose {
					status := result.Error
					if result.Status != 0 {
						status = fmt.Sprintf("%d", result.Status)
					}

					if result.Error != "" {
						log.Infof("%s[%d/%d] %s - Error: %s%s", color.YELLOW, count, len(urls), truncate(result.URL, 60), status, color.END)
					} else {
						log.Infof("%s[%d/%d] %s - HTTP %s - Buckets: %d%s", color.GREEN, count, len(urls), truncate(result.URL, 60), status, len(result.Buckets), color.END)
					}
				}
			}
		}()
	}

	// Send jobs
	for _, url := range urls {
		jobs <- url
	}
	close(jobs)

	// Wait for all workers to finish
	wg.Wait()

	log.Infof("%s[+] Completed crawling %d URL(s)%s", color.GREEN, len(urls), color.END)
	return results
}

// BucketFinder handles URL file parsing
type BucketFinder struct {
	urlsFile string
}

// NewBucketFinder creates a new BucketFinder
func NewBucketFinder(urlsFile string) *BucketFinder {
	return &BucketFinder{urlsFile: urlsFile}
}

// FileParse parses URLs from file
func (bf *BucketFinder) FileParse() ([]string, error) {
	file, err := os.Open(bf.urlsFile)
	if err != nil {
		log.Errorf("%s(-) File not found: %s%s", color.RED, bf.urlsFile, color.END)
		return nil, err
	}
	defer file.Close()

	urls := []string{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip blank lines
		if line == "" {
			continue
		}

		// Skip comments
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		// Add https:// if missing
		if !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
			line = "https://" + line
		}

		urls = append(urls, line)
	}

	if err := scanner.Err(); err != nil {
		log.Errorf("%s(-) Error reading file: %v%s", color.RED, err, color.END)
		return nil, err
	}

	log.Infof("%s[+] Successfully loaded %d URL(s) from %s%s", color.SKY_BLUE, len(urls), bf.urlsFile, color.END)
	return urls, nil
}

// PrintSummary prints summary of crawling results
func PrintSummary(results []CrawlResult) {
	// Collect statistics
	successful := 0
	for _, r := range results {
		if r.Status == 200 {
			successful++
		}
	}
	failed := len(results) - successful

	// Collect all unique buckets
	bucketsMap := make(map[string]bool)
	for _, r := range results {
		for _, bucket := range r.Buckets {
			bucketsMap[bucket] = true
		}
	}

	// Convert to sorted slice
	buckets := make([]string, 0, len(bucketsMap))
	for bucket := range bucketsMap {
		buckets = append(buckets, bucket)
	}

	// Print summary
	log.Infof("%s[+] CRAWL SUMMARY%s", color.GREEN, color.END)
	log.Infof("%s[+] Total URLs crawled: %d%s", color.GREEN, len(results), color.END)
	log.Infof("%s[+] Successful crawls: %d%s", color.GREEN, successful, color.END)
	log.Infof("%s[-] Failed crawls: %d%s", color.RED, failed, color.END)
	log.Infof("%s[+] Unique buckets found: %d%s", color.GREEN, len(buckets), color.END)

	if len(buckets) > 0 {
		log.Infof("%s[+] All unique buckets:%s", color.GREEN, color.END)
		for _, bucket := range buckets {
			log.Infof("%s    - %s%s", color.SKY_BLUE, bucket, color.END)
		}
	} else {
		log.Warnf("%s[!] No buckets found in any URLs%s", color.YELLOW, color.END)
	}

	log.Infof("%s%s%s", color.CYAN, strings.Repeat("=", 80), color.END)
}

// Helper function to truncate strings
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func main() {
	// Command-line flags
	urlsFile := flag.String("u", "", "Path to file containing URLs (one per line) (required)")
	output := flag.String("o", "", "Output file path")
	format := flag.String("f", "txt", "Output format (txt, json, csv)")
	workers := flag.Int("w", 5, "Number of concurrent workers")
	timeout := flag.Int("t", 30, "Request timeout in seconds")
	verbose := flag.Bool("v", false, "Verbose output (show progress per URL)")
	userAgent := flag.String("user-agent", "", "Custom User-Agent string")
	debug := flag.Bool("debug", false, "Enable debug logging")

	flag.Parse()

	// Configure logging
	log.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true,
		ForceColors:      true,
	})

	if *debug {
		log.SetLevel(logrus.DebugLevel)
	} else {
		log.SetLevel(logrus.InfoLevel)
	}

	// Validate required flags
	if *urlsFile == "" {
		fmt.Fprintf(os.Stderr, "Error: -u flag is required\n")
		flag.Usage()
		os.Exit(1)
	}

	log.Infof("%s%s%s", color.CYAN, strings.Repeat("=", 80), color.END)
	log.Infof("%s[+] AWS S3 Bucket Crawler Starting%s", color.GREEN, color.END)
	log.Infof("%s%s%s", color.CYAN, strings.Repeat("=", 80), color.END)

	// Load URLs
	finder := NewBucketFinder(*urlsFile)
	urls, err := finder.FileParse()
	if err != nil || len(urls) == 0 {
		log.Errorf("%s(-) No URLs loaded. Exiting.%s", color.RED, color.END)
		os.Exit(1)
	}

	// Initialize output handler
	var outputHandler *OutputFiles
	if *output != "" {
		outputHandler = NewOutputFiles(*output)
		if err := outputHandler.MakeFile(); err != nil {
			log.Errorf("%s(-) Error creating output files: %v%s", color.RED, err, color.END)
			os.Exit(1)
		}
	}

	// Initialize crawler
	crawler := NewWebCrawler(*timeout, *workers, *userAgent, outputHandler)

	// Crawl URLs
	results := crawler.CrawlURLs(urls, *verbose)

	// Print summary
	PrintSummary(results)

	// Save final results if output file specified
	if outputHandler != nil {
		if err := outputHandler.SaveResults(results, *format); err != nil {
			log.Errorf("%s(-) Error saving results: %v%s", color.RED, err, color.END)
		} else {
			log.Infof("%s[+] Results saved to %s%s", color.GREEN, outputHandler.outputFile, color.END)
		}
	}

	log.Infof("%s[+] Crawl completed successfully%s", color.GREEN, color.END)
}