package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	pkglogger "github.com/damoang/angple-backend/pkg/logger"
)

// S3Client wraps the AWS S3 client for S3/R2/MinIO compatible storage
type S3Client struct {
	client   *s3.Client
	bucket   string
	cdnURL   string // optional CDN base URL (e.g. https://cdn.angple.com)
	basePath string // prefix for all objects (e.g. "uploads/")
}

// S3Config holds S3-compatible storage configuration
type S3Config struct {
	Endpoint        string // e.g. https://xxx.r2.cloudflarestorage.com
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	CDNURL          string
	BasePath        string
	ForcePathStyle  bool // true for MinIO/R2
}

// NewS3Client creates a new S3-compatible storage client
func NewS3Client(cfg S3Config) (*S3Client, error) {
	opts := func(o *s3.Options) {
		o.Region = cfg.Region
		o.Credentials = credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.ForcePathStyle
	}

	client := s3.New(s3.Options{}, opts)

	pkglogger.GetLogger().Info().
		Str("bucket", cfg.Bucket).
		Str("endpoint", cfg.Endpoint).
		Msg("S3 storage client initialized")

	return &S3Client{
		client:   client,
		bucket:   cfg.Bucket,
		cdnURL:   strings.TrimRight(cfg.CDNURL, "/"),
		basePath: cfg.BasePath,
	}, nil
}

// UploadResult contains the result of a file upload
type UploadResult struct {
	Key         string `json:"key"`
	URL         string `json:"url"`
	CDNURL      string `json:"cdn_url,omitempty"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// Upload uploads a file to S3-compatible storage
func (c *S3Client) Upload(ctx context.Context, key string, body io.Reader, contentType string, size int64) (*UploadResult, error) {
	fullKey := c.basePath + key

	input := &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(fullKey),
		Body:        body,
		ContentType: aws.String(contentType),
	}

	if _, err := c.client.PutObject(ctx, input); err != nil {
		return nil, fmt.Errorf("s3 upload failed: %w", err)
	}

	result := &UploadResult{
		Key:         fullKey,
		URL:         fmt.Sprintf("https://%s.s3.amazonaws.com/%s", c.bucket, fullKey),
		ContentType: contentType,
		Size:        size,
	}

	if c.cdnURL != "" {
		result.CDNURL = c.cdnURL + "/" + fullKey
	}

	return result, nil
}

// Delete removes a file from storage
func (c *S3Client) Delete(ctx context.Context, key string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}

	if _, err := c.client.DeleteObject(ctx, input); err != nil {
		return fmt.Errorf("s3 delete failed: %w", err)
	}
	return nil
}

// GetPresignedURL generates a pre-signed URL for direct download
func (c *S3Client) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(c.client)

	input := &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}

	result, err := presignClient.PresignGetObject(ctx, input, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("presign failed: %w", err)
	}

	return result.URL, nil
}

// GetCDNURL returns the CDN URL for a given key, falling back to S3 URL
func (c *S3Client) GetCDNURL(key string) string {
	if c.cdnURL != "" {
		return c.cdnURL + "/" + url.PathEscape(key)
	}
	return fmt.Sprintf("https://%s.s3.amazonaws.com/%s", c.bucket, key)
}

// GenerateKey creates a unique storage key with timestamp prefix
func GenerateKey(prefix, filename string) string {
	now := time.Now()
	ext := path.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	return fmt.Sprintf("%s/%d/%02d/%02d/%s_%d%s",
		prefix, now.Year(), now.Month(), now.Day(),
		base, now.UnixMilli(), ext)
}
