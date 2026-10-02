package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
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

// NewS3Client creates a new S3-compatible storage client.
//
// When both AccessKeyID and SecretAccessKey are set they are used as static
// credentials (R2/MinIO and environments with explicit keys). When both are
// empty the AWS SDK default credential chain is used instead (environment,
// shared config, then the instance role via IMDS), so no extra secret is
// needed where the node role already grants access. Setting only one of the
// two is a misconfiguration and returns an error rather than falling back.
func NewS3Client(cfg S3Config) (*S3Client, error) {
	client, err := newAWSS3Client(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	pkglogger.GetLogger().Info().
		Str("bucket", cfg.Bucket).
		Str("endpoint", cfg.Endpoint).
		Bool("static_credentials", hasStaticCredentials(cfg)).
		Msg("S3 storage client initialized")

	return &S3Client{
		client:   client,
		bucket:   cfg.Bucket,
		cdnURL:   strings.TrimRight(cfg.CDNURL, "/"),
		basePath: cfg.BasePath,
	}, nil
}

// hasStaticCredentials reports whether both static keys are configured.
func hasStaticCredentials(cfg S3Config) bool {
	return cfg.AccessKeyID != "" && cfg.SecretAccessKey != ""
}

// newAWSS3Client builds the SDK client, choosing static keys or the default
// credential chain as described on NewS3Client.
func newAWSS3Client(ctx context.Context, cfg S3Config) (*s3.Client, error) {
	endpointOpts := func(o *s3.Options) {
		if cfg.Region != "" {
			o.Region = cfg.Region
		}
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.ForcePathStyle
	}

	if hasStaticCredentials(cfg) {
		return s3.New(s3.Options{
			Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		}, endpointOpts), nil
	}
	if cfg.AccessKeyID != "" || cfg.SecretAccessKey != "" {
		return nil, errors.New("s3 config: access key id and secret access key must be set together")
	}

	var loadOpts []func(*awsconfig.LoadOptions) error
	if cfg.Region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(cfg.Region))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("s3 config: load default credential chain: %w", err)
	}
	return s3.NewFromConfig(awsCfg, endpointOpts), nil
}

// UploadResult contains the result of a file upload
type UploadResult struct {
	Key         string `json:"key"`
	URL         string `json:"url"`
	CDNURL      string `json:"cdn_url,omitempty"`
	OriginURL   string `json:"origin_url,omitempty"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// Upload uploads a file to S3-compatible storage
func (c *S3Client) Upload(ctx context.Context, key string, body io.Reader, contentType string, size int64) (*UploadResult, error) {
	return c.UploadWithMetadata(ctx, key, body, contentType, size, nil)
}

// UploadWithMetadata uploads a file with user-defined object metadata
// (sent as x-amz-meta-* headers). A nil or empty map behaves like Upload.
func (c *S3Client) UploadWithMetadata(ctx context.Context, key string, body io.Reader, contentType string, size int64, metadata map[string]string) (*UploadResult, error) {
	fullKey := c.basePath + key

	input := &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(fullKey),
		Body:        body,
		ContentType: aws.String(contentType),
	}
	if len(metadata) > 0 {
		input.Metadata = metadata
	}

	if _, err := c.client.PutObject(ctx, input); err != nil {
		return nil, fmt.Errorf("s3 upload failed: %w", err)
	}

	originURL := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", c.bucket, fullKey)
	result := &UploadResult{
		Key:         fullKey,
		URL:         originURL,
		CDNURL:      "",
		OriginURL:   originURL,
		ContentType: contentType,
		Size:        size,
	}

	if c.cdnURL != "" {
		result.CDNURL = c.cdnURL + "/" + fullKey
		result.URL = result.CDNURL
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

// Exists reports whether an object exists. Like Upload, the key is relative to
// the configured base path. A missing object (404) returns false with a nil
// error; any other failure (including 403 when the caller lacks list
// permission) is returned as an error.
func (c *S3Client) Exists(ctx context.Context, key string) (bool, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(c.basePath + key),
	}

	if _, err := c.client.HeadObject(ctx, input); err != nil {
		var notFound *types.NotFound
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &notFound) || errors.As(err, &noSuchKey) {
			return false, nil
		}
		return false, fmt.Errorf("s3 head failed: %w", err)
	}
	return true, nil
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
