package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

		localPath, err := safeJoin(*localDir, name)
		if err != nil {
			result.failed++
			log.Printf("[path] %s failed: %v", name, err)
			continue
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// #nosec G204 -- bucket and key are validated application inputs, and CommandContext avoids shell expansion.
	cmd := exec.CommandContext(ctx, "aws", "s3api", "head-object", "--bucket", bucket, "--key", key)
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// #nosec G204 -- localPath is validated by safeJoin and the command is executed without a shell.
	cmd := exec.CommandContext(
		ctx,
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

	cleanPath := filepath.Clean(path)

	// #nosec G304 -- path is prevalidated by safeJoin and cleaned here before opening.
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

func safeJoin(baseDir, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty filename")
	}
	if filepath.Base(name) != name {
		return "", fmt.Errorf("unexpected nested path: %s", name)
	}

	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}

	fullPath := filepath.Join(baseAbs, name)
	fullAbs, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(baseAbs, fullAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes base directory: %s", name)
	}

	return fullAbs, nil
}
