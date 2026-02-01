//go:build ignore

// 이모티콘 서비스 - 파싱 로직 및 팩 관리
package service

import (
	"archive/zip"
	"fmt"
	"html"
	"io"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"angple-backend/plugins/emoticon/domain"

	"gorm.io/gorm"
)

// 허용 확장자
var allowedExtensions = map[string]bool{
	".gif":  true,
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
}

// 이모티콘 코드 정규식: {emo:filename:width} 또는 {emo:filename}
var emoPattern = regexp.MustCompile(`\{emo:([^}]+)\}`)

// Config 이모티콘 서비스 설정
type Config struct {
	DefaultWidth     int    `json:"default_width"`
	MaxWidth         int    `json:"max_width"`
	CDNURL           string `json:"cdn_url"`
	FallbackFilename string `json:"fallback_filename"`
	AssetsPath       string `json:"assets_path"`
	BaseURL          string `json:"base_url"`
}

// DefaultConfig 기본 설정값 반환
func DefaultConfig() Config {
	return Config{
		DefaultWidth:     50,
		MaxWidth:         200,
		CDNURL:           "",
		FallbackFilename: "damoang-emo-010.gif",
		AssetsPath:       "plugins/emoticon/assets",
		BaseURL:          "/api/plugins/emoticon",
	}
}

// Service 이모티콘 서비스
type Service struct {
	db     *gorm.DB
	config Config
}

// NewService 서비스 생성
func NewService(db *gorm.DB, config Config) *Service {
	return &Service{db: db, config: config}
}

// ParseContent 콘텐츠 내 {emo:...} 코드를 <img> 태그로 변환
func (s *Service) ParseContent(content string) string {
	return emoPattern.ReplaceAllStringFunc(content, func(match string) string {
		sub := emoPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		return s.renderEmoticon(sub[1])
	})
}

// renderEmoticon 단일 이모티콘 코드를 <img> 태그로 변환
func (s *Service) renderEmoticon(code string) string {
	parts := strings.SplitN(code, ":", 2)
	filename := parts[0]
	width := s.config.DefaultWidth

	if len(parts) > 1 {
		if w, err := strconv.Atoi(parts[1]); err == nil && w > 0 {
			width = w
		}
	}

	// 너비 제한
	if width > s.config.MaxWidth {
		width = s.config.MaxWidth
	}
	if width < 20 {
		width = 20
	}

	// 보안: path traversal 차단
	filename = filepath.Base(filename)
	if !isAllowedExtension(filename) {
		return s.renderFallback()
	}
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return s.renderFallback()
	}

	// 파일 존재 확인
	assetPath := filepath.Join(s.config.AssetsPath, filename)
	if _, err := os.Stat(assetPath); os.IsNotExist(err) {
		return s.renderFallback()
	}

	src := s.imageURL(filename)
	escapedSrc := html.EscapeString(src)

	return fmt.Sprintf(
		`<img src="%s" width="%d" class="emoticon" loading="lazy" alt="emoticon" />`,
		escapedSrc, width,
	)
}

// renderFallback 삭제된 이모티콘 폴백 렌더링
func (s *Service) renderFallback() string {
	src := s.imageURL(s.config.FallbackFilename)
	return fmt.Sprintf(
		`(삭제된 이모지) <img src="%s" width="%d" class="emoticon" loading="lazy" alt="deleted emoticon" />`,
		html.EscapeString(src), s.config.DefaultWidth,
	)
}

// imageURL CDN 또는 로컬 이미지 URL 생성
func (s *Service) imageURL(filename string) string {
	if s.config.CDNURL != "" {
		return s.config.CDNURL + "/emoticon/" + filename
	}
	return s.config.BaseURL + "/image/" + filename
}

// GetActivePacks 활성 팩 목록 조회
func (s *Service) GetActivePacks() ([]domain.EmoticonPack, error) {
	var packs []domain.EmoticonPack
	err := s.db.Where("is_active = ?", true).
		Order("sort_order ASC, name ASC").
		Find(&packs).Error
	return packs, err
}

// GetAllPacks 전체 팩 목록 조회 (관리용)
func (s *Service) GetAllPacks() ([]domain.EmoticonPack, error) {
	var packs []domain.EmoticonPack
	err := s.db.Order("sort_order ASC, name ASC").Find(&packs).Error
	return packs, err
}

// GetPackBySlug 슬러그로 팩 조회
func (s *Service) GetPackBySlug(slug string) (*domain.EmoticonPack, error) {
	var pack domain.EmoticonPack
	err := s.db.Where("slug = ?", slug).First(&pack).Error
	return &pack, err
}

// GetPackByID ID로 팩 조회
func (s *Service) GetPackByID(id int64) (*domain.EmoticonPack, error) {
	var pack domain.EmoticonPack
	err := s.db.First(&pack, id).Error
	return &pack, err
}

// GetItemsByPackSlug 팩 슬러그로 아이템 목록 조회
func (s *Service) GetItemsByPackSlug(slug string) ([]domain.EmoticonItem, error) {
	pack, err := s.GetPackBySlug(slug)
	if err != nil {
		return nil, err
	}

	var items []domain.EmoticonItem
	err = s.db.Where("pack_id = ? AND is_active = ?", pack.ID, true).
		Order("filename ASC").
		Find(&items).Error
	return items, err
}

// CreatePackFromZip ZIP 파일에서 팩 생성
func (s *Service) CreatePackFromZip(zipPath, slug, name string) (*domain.EmoticonPack, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("ZIP 파일 열기 실패: %w", err)
	}
	defer reader.Close()

	// 팩 생성
	pack := &domain.EmoticonPack{
		Slug:         slug,
		Name:         name,
		DefaultWidth: s.config.DefaultWidth,
		IsActive:     true,
	}
	if err := s.db.Create(pack).Error; err != nil {
		return nil, fmt.Errorf("팩 생성 실패: %w", err)
	}

	// ZIP 내 파일 추출
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		filename := filepath.Base(file.Name)
		if !isAllowedExtension(filename) {
			continue
		}

		// 파일 추출
		destPath := filepath.Join(s.config.AssetsPath, filename)
		if err := extractFile(file, destPath); err != nil {
			continue
		}

		// DB 등록
		mimeType := mime.TypeByExtension(filepath.Ext(filename))
		item := domain.EmoticonItem{
			PackID:   pack.ID,
			Filename: filename,
			MimeType: mimeType,
			IsActive: true,
		}
		s.db.Create(&item)
	}

	return pack, nil
}

// UpdatePack 팩 수정
func (s *Service) UpdatePack(id int64, updates map[string]interface{}) (*domain.EmoticonPack, error) {
	var pack domain.EmoticonPack
	if err := s.db.First(&pack, id).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&pack).Updates(updates).Error; err != nil {
		return nil, err
	}
	s.db.First(&pack, id)
	return &pack, nil
}

// DeletePack 팩 삭제 (아이템 + 파일 포함)
func (s *Service) DeletePack(id int64) error {
	// 아이템 파일 삭제
	var items []domain.EmoticonItem
	s.db.Where("pack_id = ?", id).Find(&items)
	for _, item := range items {
		os.Remove(filepath.Join(s.config.AssetsPath, item.Filename))
	}

	// DB 삭제 (CASCADE로 아이템도 삭제됨)
	return s.db.Delete(&domain.EmoticonPack{}, id).Error
}

// TogglePack 팩 활성/비활성 토글
func (s *Service) TogglePack(id int64) (*domain.EmoticonPack, error) {
	var pack domain.EmoticonPack
	if err := s.db.First(&pack, id).Error; err != nil {
		return nil, err
	}
	pack.IsActive = !pack.IsActive
	if err := s.db.Save(&pack).Error; err != nil {
		return nil, err
	}
	return &pack, nil
}

// GetAssetPath 이미지 파일 경로 반환 (보안 검증 포함)
func (s *Service) GetAssetPath(filename string) (string, error) {
	filename = filepath.Base(filename)
	if !isAllowedExtension(filename) {
		return "", fmt.Errorf("허용되지 않는 파일 형식")
	}

	fullPath := filepath.Join(s.config.AssetsPath, filename)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return "", fmt.Errorf("파일을 찾을 수 없음")
	}

	return fullPath, nil
}

// ImportLegacy ang-gnu 레거시 이모티콘 디렉토리에서 팩/아이템 일괄 임포트
func (s *Service) ImportLegacy(legacyDir string) (int, int, error) {
	entries, err := os.ReadDir(legacyDir)
	if err != nil {
		return 0, 0, fmt.Errorf("레거시 디렉토리 읽기 실패: %w", err)
	}

	packMap := map[string]*domain.EmoticonPack{}
	packsCreated := 0
	itemsCreated := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if !isAllowedExtension(filename) {
			continue
		}
		// 썸네일 파일 건너뛰기
		if strings.Contains(filename, "_thumb") {
			continue
		}

		// 파일명에서 팩 슬러그 추출: damoang-air-001.gif → damoang-air
		baseName := strings.TrimSuffix(filename, filepath.Ext(filename))
		parts := strings.Split(baseName, "-")
		if len(parts) < 3 {
			continue
		}
		// 마지막 파트(번호)를 제외한 나머지가 슬러그
		slug := strings.Join(parts[:len(parts)-1], "-")

		// 팩 생성 (최초 1회)
		pack, exists := packMap[slug]
		if !exists {
			pack = &domain.EmoticonPack{
				Slug:         slug,
				Name:         slug,
				DefaultWidth: s.config.DefaultWidth,
				IsActive:     true,
			}
			// 이미 DB에 존재하면 조회
			var existing domain.EmoticonPack
			if err := s.db.Where("slug = ?", slug).First(&existing).Error; err == nil {
				pack = &existing
			} else {
				if err := s.db.Create(pack).Error; err != nil {
					continue
				}
				packsCreated++
			}
			packMap[slug] = pack
		}

		// 파일 복사
		srcPath := filepath.Join(legacyDir, filename)
		destPath := filepath.Join(s.config.AssetsPath, filename)
		if err := copyFile(srcPath, destPath); err != nil {
			continue
		}

		// 썸네일 복사 (존재하면)
		thumbName := findThumbFile(legacyDir, baseName)
		thumbPath := ""
		if thumbName != "" {
			thumbSrc := filepath.Join(legacyDir, thumbName)
			thumbDest := filepath.Join(s.config.AssetsPath, thumbName)
			if err := copyFile(thumbSrc, thumbDest); err == nil {
				thumbPath = thumbName
			}
		}

		// DB에 아이템이 이미 존재하면 건너뛰기
		var count int64
		s.db.Model(&domain.EmoticonItem{}).Where("filename = ?", filename).Count(&count)
		if count > 0 {
			continue
		}

		mimeType := mime.TypeByExtension(filepath.Ext(filename))
		item := domain.EmoticonItem{
			PackID:    pack.ID,
			Filename:  filename,
			ThumbPath: thumbPath,
			MimeType:  mimeType,
			IsActive:  true,
		}
		if err := s.db.Create(&item).Error; err == nil {
			itemsCreated++
		}
	}

	return packsCreated, itemsCreated, nil
}

// findThumbFile 레거시 디렉토리에서 썸네일 파일 찾기
func findThumbFile(dir, baseName string) string {
	for _, ext := range []string{".webp", ".gif", ".png", ".jpg"} {
		thumbName := baseName + "_thumb" + ext
		if _, err := os.Stat(filepath.Join(dir, thumbName)); err == nil {
			return thumbName
		}
	}
	return ""
}

// copyFile 파일 복사
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// isAllowedExtension 허용 확장자 확인
func isAllowedExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return allowedExtensions[ext]
}

// extractFile ZIP 파일에서 단일 파일 추출
func extractFile(file *zip.File, destPath string) error {
	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	outFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// 파일 크기 제한 (10MB)
	_, err = io.Copy(outFile, io.LimitReader(rc, 10*1024*1024))
	return err
}
