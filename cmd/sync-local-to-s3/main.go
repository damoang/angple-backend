package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	defaultBucket = "damoang-data-v1"
	defaultRoot   = "/home/damoang/www"
)

type config struct {
	localDir     string
	bucket       string
	prefix       string
	root         string
	exclude      string
	match        string
	limit        int
	concurrency  int
	apply        bool
	recursive    bool
	reportFile   string
	cacheControl string
}

type result struct {
	Checked  int64    `json:"checked"`
	Existing int64    `json:"existing"`
	Missing  int64    `json:"missing"`
	Uploaded int64    `json:"uploaded"`
	Failed   int64    `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
}

type fileEntry struct {
	localPath string
	s3Key     string
}

var s3client *s3.Client

func main() {
	cfg := parseFlags()

	if cfg.localDir == "" {
		log.Fatal("--dir is required")
	}

	// Resolve prefix from dir if not specified
	if cfg.prefix == "" {
		rel, err := filepath.Rel(cfg.root, cfg.localDir)
		if err != nil {
			log.Fatalf("cannot resolve prefix: dir %s is not under root %s", cfg.localDir, cfg.root)
		}
		cfg.prefix = filepath.ToSlash(rel)
	}

	// Initialize AWS SDK S3 client
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("ap-northeast-2"),
	)
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	s3client = s3.NewFromConfig(awsCfg)

	log.Printf("[config] dir=%s bucket=%s prefix=%s recursive=%v apply=%v concurrency=%d",
		cfg.localDir, cfg.bucket, cfg.prefix, cfg.recursive, cfg.apply, cfg.concurrency)

	files, err := collectFiles(cfg)
	if err != nil {
		log.Fatalf("failed to collect files: %v", err)
	}

	log.Printf("[scan] found %d files to check", len(files))

	start := time.Now()
	res := processFiles(cfg, files)
	elapsed := time.Since(start)

	log.Printf("[summary] checked=%d existing=%d missing=%d uploaded=%d failed=%d apply=%v elapsed=%s",
		res.Checked, res.Existing, res.Missing, res.Uploaded, res.Failed, cfg.apply, elapsed.Round(time.Second))

	if cfg.reportFile != "" {
		writeReport(cfg.reportFile, res)
	}

	if res.Failed > 0 {
		os.Exit(1)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.localDir, "dir", "", "local source directory (required)")
	flag.StringVar(&cfg.bucket, "bucket", defaultBucket, "target S3 bucket")
	flag.StringVar(&cfg.prefix, "prefix", "", "S3 key prefix (default: auto from --dir relative to --root)")
	flag.StringVar(&cfg.root, "root", defaultRoot, "local web root for auto-prefix resolution")
	flag.StringVar(&cfg.exclude, "exclude", "", "comma-separated substrings to exclude (e.g. _none,.php)")
	flag.StringVar(&cfg.match, "match", "", "only process files containing this substring")
	flag.IntVar(&cfg.limit, "limit", 0, "max files to process (0 = unlimited)")
	flag.IntVar(&cfg.concurrency, "concurrency", 50, "parallel S3 checks/uploads")
	flag.BoolVar(&cfg.apply, "apply", false, "actually upload missing files (default: dry-run)")
	flag.BoolVar(&cfg.recursive, "recursive", true, "recurse into subdirectories")
	flag.StringVar(&cfg.reportFile, "report", "", "write JSON report to file")
	flag.StringVar(&cfg.cacheControl, "cache-control", "public, max-age=31536000, immutable", "Cache-Control header")
	flag.Parse()
	return cfg
}

func collectFiles(cfg config) ([]fileEntry, error) {
	var files []fileEntry
	excludes := splitCSV(cfg.exclude)

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if !cfg.recursive && path != cfg.localDir {
				return fs.SkipDir
			}
			return nil
		}

		name := d.Name()

		// Apply exclude filter
		for _, ex := range excludes {
			if strings.Contains(name, ex) {
				return nil
			}
		}

		// Apply match filter
		if cfg.match != "" && !strings.Contains(name, cfg.match) {
			return nil
		}

		// Compute S3 key
		rel, err := filepath.Rel(cfg.localDir, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(filepath.Join(cfg.prefix, rel))

		files = append(files, fileEntry{localPath: path, s3Key: key})

		if cfg.limit > 0 && len(files) >= cfg.limit {
			return fmt.Errorf("limit reached")
		}

		return nil
	}

	err := filepath.WalkDir(cfg.localDir, walkFn)
	if err != nil && err.Error() != "limit reached" {
		return nil, err
	}

	return files, nil
}

func processFiles(cfg config, files []fileEntry) result {
	var res result

	sem := make(chan struct{}, cfg.concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Progress logging
	total := int64(len(files))
	var lastLog time.Time

	for _, f := range files {
		wg.Add(1)
		sem <- struct{}{}

		go func(entry fileEntry) {
			defer wg.Done()
			defer func() { <-sem }()

			checked := atomic.AddInt64(&res.Checked, 1)

			// Log progress every 5 seconds
			now := time.Now()
			mu.Lock()
			if now.Sub(lastLog) > 5*time.Second {
				lastLog = now
				log.Printf("[progress] %d/%d checked", checked, total)
			}
			mu.Unlock()

			exists, err := objectExists(cfg.bucket, entry.s3Key)
			if err != nil {
				atomic.AddInt64(&res.Failed, 1)
				mu.Lock()
				res.Errors = append(res.Errors, fmt.Sprintf("[check] %s: %v", entry.s3Key, err))
				mu.Unlock()
				log.Printf("[error] check %s: %v", entry.s3Key, err)
				return
			}

			if exists {
				atomic.AddInt64(&res.Existing, 1)
				return
			}

			atomic.AddInt64(&res.Missing, 1)
			log.Printf("[missing] %s", entry.s3Key)

			if !cfg.apply {
				return
			}

			if err := uploadFile(entry.localPath, cfg.bucket, entry.s3Key, cfg.cacheControl); err != nil {
				atomic.AddInt64(&res.Failed, 1)
				mu.Lock()
				res.Errors = append(res.Errors, fmt.Sprintf("[upload] %s: %v", entry.s3Key, err))
				mu.Unlock()
				log.Printf("[error] upload %s: %v", entry.s3Key, err)
				return
			}

			atomic.AddInt64(&res.Uploaded, 1)
			log.Printf("[uploaded] %s", entry.s3Key)
		}(f)
	}

	wg.Wait()
	return res
}

func objectExists(bucket, key string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := s3client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}

	// Check for 404 Not Found
	errStr := err.Error()
	if strings.Contains(errStr, "NotFound") || strings.Contains(errStr, "404") || strings.Contains(errStr, "NoSuchKey") {
		return false, nil
	}

	return false, err
}

func uploadFile(localPath, bucket, key, cacheControl string) error {
	contentType, err := detectContentType(localPath)
	if err != nil {
		return err
	}

	cleanPath := filepath.Clean(localPath)

	// #nosec G304 -- path comes from filepath.WalkDir on a known directory
	file, err := os.Open(cleanPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	_, err = s3client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(bucket),
		Key:          aws.String(key),
		Body:         file,
		ContentType:  aws.String(contentType),
		CacheControl: aws.String(cacheControl),
	})
	if err != nil {
		return fmt.Errorf("s3 put: %w", err)
	}
	return nil
}

func detectContentType(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct, nil
	}

	cleanPath := filepath.Clean(path)

	// #nosec G304 -- path comes from filepath.WalkDir on a known directory
	file, err := os.Open(cleanPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, err := bufio.NewReader(file).Read(buf)
	if err != nil && err.Error() != "EOF" {
		return "", err
	}

	return http.DetectContentType(bytes.TrimSpace(buf[:n])), nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func writeReport(path string, res result) {
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		log.Printf("[report] failed to marshal: %v", err)
		return
	}

	cleanPath := filepath.Clean(path)
	if err := os.WriteFile(cleanPath, data, 0600); err != nil {
		log.Printf("[report] failed to write %s: %v", cleanPath, err)
		return
	}
	log.Printf("[report] written to %s", cleanPath)
}
