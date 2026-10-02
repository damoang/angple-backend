package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	pkglogger "github.com/damoang/angple-backend/pkg/logger"
	"github.com/damoang/angple-backend/pkg/storage"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
)

const (
	// maxProfileImageSize 는 프로필 원본 업로드 상한이다. 서버에서 줄이지 않고
	// 변환 람다가 축소하므로 휴대폰 원본 사진이 들어올 수 있는 크기로 둔다.
	maxProfileImageSize = 5 * 1024 * 1024 // 5MB

	// memberImageRawPrefix 는 원본을 올리는 경로다. 이 경로 업로드가 변환 람다를 깨운다.
	// memberImageDataPrefix 는 변환 람다가 webp 결과를 쓰는 경로다.
	memberImageRawPrefix  = "raw/member_image/"
	memberImageDataPrefix = "data/member_image/"

	// memberImageMetaKey/Value 는 변환 람다가 프로필 사진 분기를 타게 하는 객체 메타데이터다.
	// 빠지면 일반 이미지로 취급되어 webp·크기별 변형이 만들어지지 않는다.
	memberImageMetaKey   = "Photo-Type"
	memberImageMetaValue = "member-profile"

	// 변환 결과(.webp)를 기다리는 간격과 상한. 큰 GIF 는 변환에 수 초가 걸려
	// 상한을 실측 최대치보다 넉넉하게 둔다. 웹 프록시 타임아웃보다 충분히 짧다.
	defaultMemberImagePollInterval = 300 * time.Millisecond
	defaultMemberImagePollTimeout  = 12 * time.Second

	// memberImageKeyRetries 는 같은 초에 다시 올려 원본 키가 겹칠 때 시각을 밀어 보는 횟수다.
	memberImageKeyRetries = 3

	errMsgProfileImageTooLarge = "파일 크기가 너무 큽니다 (최대 5MB)"
	errMsgProfileImageFormat   = "지원하지 않는 이미지 형식입니다 (jpg, png, gif, webp 만 가능)"
)

// ErrMemberImageProcessingTimeout is returned when the converted profile image
// did not appear within the polling window (12s by default). The DB is left
// unchanged.
var ErrMemberImageProcessingTimeout = errors.New("이미지 처리가 지연되고 있습니다. 잠시 후 다시 시도해 주세요")

// MemberImageValidationError is returned when an uploaded profile image is
// rejected by size or format validation. Its message is safe to show users.
type MemberImageValidationError struct {
	Msg string
}

// Error implements the error interface.
func (e *MemberImageValidationError) Error() string {
	return e.Msg
}

// memberImageStore is the subset of storage used for profile images.
type memberImageStore interface {
	UploadWithMetadata(ctx context.Context, key string, body io.Reader, contentType string, size int64, metadata map[string]string) (*storage.UploadResult, error)
	Exists(ctx context.Context, key string) (bool, error)
}

// MemberService handles member profile image operations
type MemberService struct {
	store        memberImageStore
	memberRepo   gnurepo.MemberRepository
	now          func() time.Time
	pollInterval time.Duration
	pollTimeout  time.Duration
}

// NewMemberService creates a new MemberService
func NewMemberService(s3Client *storage.S3Client, memberRepo gnurepo.MemberRepository) *MemberService {
	svc := &MemberService{
		memberRepo:   memberRepo,
		now:          time.Now,
		pollInterval: defaultMemberImagePollInterval,
		pollTimeout:  defaultMemberImagePollTimeout,
	}
	if s3Client != nil {
		svc.store = s3Client
	}
	return svc
}

// UpdateMemberImage uploads the original profile image for lambda conversion.
//
// The validated original bytes go to raw/member_image/ with the profile
// metadata. The conversion lambda then writes a .webp under data/member_image/,
// which in turn triggers the size variants. This method waits for that .webp
// and only then stores its key in the DB. Nothing is deleted from storage.
func (s *MemberService) UpdateMemberImage(ctx context.Context, mbID string, file *multipart.FileHeader) (string, error) {
	if file.Size > maxProfileImageSize {
		return "", &MemberImageValidationError{Msg: errMsgProfileImageTooLarge}
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("파일 열기 실패: %w", err)
	}
	defer src.Close()

	// 헤더의 Size 를 믿지 않고 상한+1 까지만 읽는다.
	data, err := io.ReadAll(io.LimitReader(src, maxProfileImageSize+1))
	if err != nil {
		return "", fmt.Errorf("파일 읽기 실패: %w", err)
	}

	return s.uploadMemberImage(ctx, mbID, file.Filename, data)
}

func (s *MemberService) uploadMemberImage(ctx context.Context, mbID, filename string, data []byte) (string, error) {
	if s.store == nil {
		return "", errors.New("스토리지가 설정되지 않았습니다")
	}

	ext, contentType, err := validateProfileImage(filename, data)
	if err != nil {
		return "", err
	}

	rawKey, dataKey, err := s.newMemberImageKeys(ctx, mbID, ext)
	if err != nil {
		return "", err
	}

	size := int64(len(data))
	result, err := s.store.UploadWithMetadata(ctx, rawKey, bytes.NewReader(data), contentType, size,
		map[string]string{memberImageMetaKey: memberImageMetaValue})
	if err != nil {
		pkglogger.GetLogger().Error().
			Str("mb_id", mbID).
			Str("raw_key", rawKey).
			Err(err).
			Msg("member profile image upload failed")
		return "", fmt.Errorf("S3 업로드 실패: %w", err)
	}

	if err := s.waitForConvertedImage(ctx, dataKey); err != nil {
		pkglogger.GetLogger().Warn().
			Str("mb_id", mbID).
			Str("raw_key", result.Key).
			Str("data_key", dataKey).
			Err(err).
			Msg("member profile image conversion not ready; DB unchanged")
		return "", err
	}

	// 저장소 base path 가 있으면 업로드 결과 키와 같은 형태(base path 포함)로 저장한다.
	basePath := strings.TrimSuffix(result.Key, rawKey)
	storedKey := basePath + dataKey

	if err := s.memberRepo.UpdateMemberImageUrl(mbID, storedKey); err != nil {
		pkglogger.GetLogger().Error().
			Str("mb_id", mbID).
			Str("key", storedKey).
			Err(err).
			Msg("member profile image DB update failed")
		return "", fmt.Errorf("DB 업데이트 실패: %w", err)
	}

	publicURL := result.CDNURL
	if publicURL == "" {
		publicURL = result.URL
	}
	publicURL = strings.TrimSuffix(publicURL, result.Key) + storedKey

	pkglogger.GetLogger().Info().
		Str("mb_id", mbID).
		Str("raw_key", result.Key).
		Str("key", storedKey).
		Int64("size", size).
		Msg("member profile image uploaded")

	return publicURL, nil
}

// newMemberImageKeys picks a timestamp whose raw and converted keys are both
// free, so a second upload within the same second neither overwrites the
// earlier original nor mistakes the earlier .webp for its own result.
func (s *MemberService) newMemberImageKeys(ctx context.Context, mbID, ext string) (string, string, error) {
	ts := s.now().Unix()
	for i := 0; i < memberImageKeyRetries; i++ {
		rawKey, dataKey, err := memberImageKeys(mbID, ts, ext)
		if err != nil {
			return "", "", err
		}
		// 원본 확장자가 달라도 webp 키는 같은 이름이 되므로 둘 다 확인한다.
		// 확인 실패는 겹치지 않은 것으로 본다(같은 초 재업로드만 막으려는 장치).
		if !s.keyTaken(ctx, rawKey) && !s.keyTaken(ctx, dataKey) {
			return rawKey, dataKey, nil
		}
		ts++
	}
	return "", "", errors.New("잠시 후 다시 시도해 주세요")
}

// keyTaken reports whether key exists; a failed check counts as not taken.
func (s *MemberService) keyTaken(ctx context.Context, key string) bool {
	exists, err := s.store.Exists(ctx, key)
	return err == nil && exists
}

// waitForConvertedImage polls until dataKey exists, the timeout passes, or ctx ends.
func (s *MemberService) waitForConvertedImage(ctx context.Context, dataKey string) error {
	deadline := time.NewTimer(s.pollTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			if lastErr != nil {
				pkglogger.GetLogger().Warn().
					Str("data_key", dataKey).
					Err(lastErr).
					Msg("member profile image head check failed while polling")
			}
			return ErrMemberImageProcessingTimeout
		case <-ticker.C:
			exists, err := s.store.Exists(ctx, dataKey)
			if err != nil {
				// 권한상 없는 객체가 403 으로 올 수 있어 「아직 없음」으로 보고 계속 본다.
				lastErr = err
				continue
			}
			if exists {
				return nil
			}
		}
	}
}

// memberImageKeys builds the raw upload key and the expected converted key.
// The folder is the first two characters of mb_id as-is (case preserved);
// a one-character mb_id uses that single character.
func memberImageKeys(mbID string, ts int64, ext string) (string, string, error) {
	if mbID == "" || strings.Contains(mbID, "/") {
		return "", "", errors.New("잘못된 회원 ID 입니다")
	}

	prefix := mbID
	if r := []rune(mbID); len(r) > 2 {
		prefix = string(r[:2])
	}

	name := fmt.Sprintf("%s/%s_%d", prefix, mbID, ts)
	return memberImageRawPrefix + name + ext, memberImageDataPrefix + name + ".webp", nil
}

// validateProfileImage checks size, extension and sniffed content type.
// It returns the extension to use (the sniffed format's when the filename's
// extension does not match) and the content type.
func validateProfileImage(filename string, data []byte) (string, string, error) {
	if len(data) > maxProfileImageSize {
		return "", "", &MemberImageValidationError{Msg: errMsgProfileImageTooLarge}
	}
	if len(data) == 0 {
		return "", "", &MemberImageValidationError{Msg: "빈 파일입니다"}
	}

	ext := strings.ToLower(path.Ext(filename))
	if !isProfileImageExt(ext) {
		return "", "", &MemberImageValidationError{Msg: errMsgProfileImageFormat}
	}

	contentType := http.DetectContentType(data)
	actualExt, ok := profileImageExtByContentType(contentType)
	if !ok {
		return "", "", &MemberImageValidationError{Msg: errMsgProfileImageFormat}
	}

	if !sameProfileImageFormat(ext, actualExt) {
		ext = actualExt
	}
	return ext, contentType, nil
}

// profileImageExtByContentType maps an allowed sniffed content type to its extension.
func profileImageExtByContentType(contentType string) (string, bool) {
	switch contentType {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/gif":
		return ".gif", true
	case "image/webp":
		return ".webp", true
	}
	return "", false
}

func sameProfileImageFormat(ext, actualExt string) bool {
	if ext == actualExt {
		return true
	}
	return ext == ".jpeg" && actualExt == ".jpg"
}

// DeleteMemberImage clears a member's profile image in the DB.
// The stored objects are kept (originals are preserved).
func (s *MemberService) DeleteMemberImage(_ context.Context, mbID string) error {
	if err := s.memberRepo.ClearMemberImageUrl(mbID); err != nil {
		pkglogger.GetLogger().Error().
			Str("mb_id", mbID).
			Err(err).
			Msg("member profile image clear failed")
		return fmt.Errorf("DB 업데이트 실패: %w", err)
	}

	pkglogger.GetLogger().Info().
		Str("mb_id", mbID).
		Msg("member profile image cleared (storage objects kept)")

	return nil
}

func isProfileImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	}
	return false
}
