package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	defaultLocalDir = "/home/damoang/www/data/nariya/image"
	defaultBucket   = "damoang-data-v1"
	defaultPrefix   = "data/nariya/image"
)

type syncResult struct {
	checked  int
	missing  int
	uploaded int
	failed   int
}

func main() {
	localDir := flag.String("dir", defaultLocalDir, "local nariya image directory")
	bucket := flag.String("bucket", defaultBucket, "target S3 bucket")
	prefix := flag.String("prefix", defaultPrefix, "target S3 prefix")
	match := flag.String("match", "", "only process files containing this substring")
	limit := flag.Int("limit", 0, "maximum number of files to process (0 = unlimited)")
	apply := flag.Bool("apply", false, "upload missing files")
	flag.Parse()

	entries, err := os.ReadDir(*localDir)
	if err != nil {
		log.Fatalf("failed to read directory: %v", err)
	}

	result := syncResult{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if *match != "" && !strings.Contains(name, *match) {
			continue
		}

		result.checked++
		if *limit > 0 && result.checked > *limit {
			break
		}

		localPath := filepath.Join(*localDir, name)
		key := strings.TrimPrefix(filepath.ToSlash(filepath.Join(*prefix, name)), "/")

		exists, err := objectExists(*bucket, key)
		if err != nil {
			result.failed++
			log.Printf("[check] %s failed: %v", key, err)
			continue
		}
		if exists {
			log.Printf("[exists] %s", key)
			continue
		}

		result.missing++
		log.Printf("[missing] %s", key)

		if !*apply {
			continue
		}

		if err := uploadFile(localPath, *bucket, key); err != nil {
			result.failed++
			log.Printf("[upload] %s failed: %v", key, err)
			continue
		}

		result.uploaded++
		log.Printf("[upload] %s uploaded", key)
	}

	log.Printf("[summary] checked=%d missing=%d uploaded=%d failed=%d apply=%v",
		result.checked, result.missing, result.uploaded, result.failed, *apply)
}

func objectExists(bucket, key string) (bool, error) {
	cmd := exec.Command("aws", "s3api", "head-object", "--bucket", bucket, "--key", key)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}

	text := string(output)
	if strings.Contains(text, "Not Found") || strings.Contains(text, "404") {
		return false, nil
	}

	return false, fmt.Errorf("%w: %s", err, strings.TrimSpace(text))
}

func uploadFile(localPath, bucket, key string) error {
	contentType, err := detectContentType(localPath)
	if err != nil {
		return err
	}

	cmd := exec.Command(
		"aws", "s3", "cp", localPath, fmt.Sprintf("s3://%s/%s", bucket, key),
		"--cache-control", "public, max-age=31536000, immutable",
		"--content-type", contentType,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func detectContentType(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if contentType := mime.TypeByExtension(ext); contentType != "" {
		return contentType, nil
	}

	file, err := os.Open(path)
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
