package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	pkglogger "github.com/damoang/angple-backend/pkg/logger"
	"github.com/damoang/angple-backend/pkg/storage"
)

// MediaService handles file uploads with image processing and S3 storage
type MediaService struct {
	s3        *storage.S3Client
	maxSize   int64    // max file size in bytes
	allowExts []string // allowed file extensions
}

// NewMediaService creates a new MediaService
func NewMediaService(s3Client *storage.S3Client) *MediaService {
	return &MediaService{
		s3:      s3Client,
		maxSize: 50 * 1024 * 1024, // 50MB
		allowExts: []string{
			".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg",
			".mp4", ".webm", ".mov",
			".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
			".zip", ".rar", ".7z", ".tar", ".gz",
			".txt", ".csv", ".json",
		},
	}
}

// UploadResult represents the result of an upload operation
type MediaUploadResult struct {
	Key         string `json:"key"`
	URL         string `json:"url"`
	CDNURL      string `json:"cdn_url,omitempty"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

// UploadImage uploads an image, optionally converting to JPEG and resizing
func (s *MediaService) UploadImage(ctx context.Context, file *multipart.FileHeader, maxWidth int) (*MediaUploadResult, error) {
	if file.Size > s.maxSize {
		return nil, fmt.Errorf("file too large (max %dMB)", s.maxSize/(1024*1024))
	}

	ext := strings.ToLower(path.Ext(file.Filename))
	if !isImageExt(ext) {
		return nil, fmt.Errorf("unsupported image format: %s", ext)
	}

	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	// Read file content
	data, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}

	contentType := http.DetectContentType(data)
	var reader io.Reader = bytes.NewReader(data)
	size := int64(len(data))
	var width, height int

	// Try to decode and resize if it's a raster image (not SVG/GIF)
	if ext != ".svg" && ext != ".gif" {
		img, format, decErr := image.Decode(bytes.NewReader(data))
		if decErr == nil {
			bounds := img.Bounds()
			width = bounds.Dx()
			height = bounds.Dy()

			// Resize if wider than maxWidth
			if maxWidth > 0 && width > maxWidth {
				img = resizeImage(img, maxWidth)
				bounds = img.Bounds()
				width = bounds.Dx()
				height = bounds.Dy()
			}

			// Re-encode
			var buf bytes.Buffer
			switch format {
			case "png":
				if err := png.Encode(&buf, img); err == nil {
					reader = &buf
					size = int64(buf.Len())
					contentType = "image/png"
					ext = ".png"
				}
			default:
				// Encode as JPEG for everything else
				if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err == nil {
					reader = &buf
					size = int64(buf.Len())
					contentType = "image/jpeg"
					ext = ".jpg"
				}
			}
		}
	}

	key := storage.GenerateKey("images", sanitizeFilename(file.Filename, ext))

	result, err := s.s3.Upload(ctx, key, reader, contentType, size)
	if err != nil {
		return nil, err
	}

	pkglogger.GetLogger().Info().
		Str("key", result.Key).
		Int64("size", size).
		Msg("image uploaded")

	return &MediaUploadResult{
		Key:         result.Key,
		URL:         result.URL,
		CDNURL:      result.CDNURL,
		Filename:    file.Filename,
		ContentType: contentType,
		Size:        size,
		Width:       width,
		Height:      height,
	}, nil
}

// UploadAttachment uploads a general file attachment
func (s *MediaService) UploadAttachment(ctx context.Context, file *multipart.FileHeader) (*MediaUploadResult, error) {
	if file.Size > s.maxSize {
		return nil, fmt.Errorf("file too large (max %dMB)", s.maxSize/(1024*1024))
	}

	ext := strings.ToLower(path.Ext(file.Filename))
	if !s.isAllowedExt(ext) {
		return nil, fmt.Errorf("file type not allowed: %s", ext)
	}

	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	// Detect content type from first 512 bytes
	buf := make([]byte, 512)
	n, readErr := src.Read(buf)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, fmt.Errorf("failed to read file header: %w", readErr)
	}
	contentType := http.DetectContentType(buf[:n])

	// Check for dangerous content types
	if isDangerousContentType(contentType) {
		return nil, fmt.Errorf("potentially dangerous file type detected")
	}

	// Reset reader
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to reset file reader: %w", err)
	}

	key := storage.GenerateKey("attachments", sanitizeFilename(file.Filename, ext))

	result, err := s.s3.Upload(ctx, key, src, contentType, file.Size)
	if err != nil {
		return nil, err
	}

	pkglogger.GetLogger().Info().
		Str("key", result.Key).
		Int64("size", file.Size).
		Str("content_type", contentType).
		Msg("attachment uploaded")

	return &MediaUploadResult{
		Key:         result.Key,
		URL:         result.URL,
		CDNURL:      result.CDNURL,
		Filename:    file.Filename,
		ContentType: contentType,
		Size:        file.Size,
	}, nil
}

// UploadVideo uploads a video file
func (s *MediaService) UploadVideo(ctx context.Context, file *multipart.FileHeader) (*MediaUploadResult, error) {
	maxVideoSize := int64(500 * 1024 * 1024) // 500MB for video
	if file.Size > maxVideoSize {
		return nil, fmt.Errorf("video too large (max %dMB)", maxVideoSize/(1024*1024))
	}

	ext := strings.ToLower(path.Ext(file.Filename))
	if !isVideoExt(ext) {
		return nil, fmt.Errorf("unsupported video format: %s", ext)
	}

	src, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	contentType := "video/" + strings.TrimPrefix(ext, ".")
	if ext == ".mov" {
		contentType = "video/quicktime"
	}

	key := storage.GenerateKey("videos", sanitizeFilename(file.Filename, ext))

	result, err := s.s3.Upload(ctx, key, src, contentType, file.Size)
	if err != nil {
		return nil, err
	}

	pkglogger.GetLogger().Info().
		Str("key", result.Key).
		Int64("size", file.Size).
		Msg("video uploaded")

	return &MediaUploadResult{
		Key:         result.Key,
		URL:         result.URL,
		CDNURL:      result.CDNURL,
		Filename:    file.Filename,
		ContentType: contentType,
		Size:        file.Size,
	}, nil
}

// DeleteFile removes a file from storage
func (s *MediaService) DeleteFile(ctx context.Context, key string) error {
	return s.s3.Delete(ctx, key)
}

// GetCDNURL returns the CDN URL for a storage key
func (s *MediaService) GetCDNURL(key string) string {
	return s.s3.GetCDNURL(key)
}

func (s *MediaService) isAllowedExt(ext string) bool {
	for _, a := range s.allowExts {
		if a == ext {
			return true
		}
	}
	return false
}

func isImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".svg":
		return true
	}
	return false
}

func isVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".webm", ".mov":
		return true
	}
	return false
}

func isDangerousContentType(ct string) bool {
	dangerous := []string{
		"application/x-executable",
		"application/x-sharedlib",
		"application/x-mach-binary",
		"application/x-dosexec",
	}
	for _, d := range dangerous {
		if strings.HasPrefix(ct, d) {
			return true
		}
	}
	return false
}

func sanitizeFilename(original, ext string) string {
	base := strings.TrimSuffix(path.Base(original), path.Ext(original))
	// Keep only alphanumeric, Korean, dash, underscore
	var result strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || (r >= 0xAC00 && r <= 0xD7A3) { // Korean
			result.WriteRune(r)
		}
	}
	s := result.String()
	if s == "" {
		s = "file"
	}
	return s + ext
}

// resizeImage resizes an image to the given max width, preserving aspect ratio
func resizeImage(img image.Image, maxWidth int) image.Image {
	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()

	if origWidth <= maxWidth {
		return img
	}

	newWidth := maxWidth
	newHeight := origHeight * newWidth / origWidth

	// Simple nearest-neighbor resize (good enough for thumbnails)
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := x * origWidth / newWidth
			srcY := y * origHeight / newHeight
			dst.Set(x, y, img.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	return dst
}
