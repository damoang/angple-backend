package service

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// FileService business logic for file uploads/downloads
type FileService interface {
	UploadEditorImage(file *multipart.FileHeader, boardID string, wrID int) (*domain.FileUploadResponse, error)
	UploadAttachment(file *multipart.FileHeader, boardID string, wrID int) (*domain.FileUploadResponse, error)
	GetFileForDownload(boardID string, wrID int, fileNo int) (*domain.FileDownloadInfo, error)
}

type fileService struct {
	repo       repository.FileRepository
	uploadPath string
}

// NewFileService creates a new FileService
func NewFileService(repo repository.FileRepository, uploadPath string) FileService {
	return &fileService{
		repo:       repo,
		uploadPath: uploadPath,
	}
}

// UploadEditorImage handles editor image uploads
func (s *fileService) UploadEditorImage(file *multipart.FileHeader, boardID string, wrID int) (*domain.FileUploadResponse, error) {
	// 이미지 파일 검증
	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowedExts := map[string]bool{extJPG: true, extJPEG: true, extPNG: true, extGIF: true, extWebP: true, ".bmp": true}
	if !allowedExts[ext] {
		return nil, fmt.Errorf("허용되지 않는 이미지 형식입니다: %s", ext)
	}

	// 파일 크기 제한 (10MB)
	if file.Size > 10*1024*1024 {
		return nil, fmt.Errorf("파일 크기가 10MB를 초과합니다")
	}

	return s.saveFile(file, boardID, wrID, true)
}

// UploadAttachment handles attachment file uploads
func (s *fileService) UploadAttachment(file *multipart.FileHeader, boardID string, wrID int) (*domain.FileUploadResponse, error) {
	// 파일 크기 제한 (50MB)
	if file.Size > 50*1024*1024 {
		return nil, fmt.Errorf("파일 크기가 50MB를 초과합니다")
	}

	// 위험한 확장자 차단
	ext := strings.ToLower(filepath.Ext(file.Filename))
	blockedExts := map[string]bool{".exe": true, ".bat": true, ".cmd": true, ".sh": true, ".php": true, ".jsp": true, ".asp": true}
	if blockedExts[ext] {
		return nil, fmt.Errorf("허용되지 않는 파일 형식입니다: %s", ext)
	}

	return s.saveFile(file, boardID, wrID, false)
}

// GetFileForDownload retrieves file info for download
func (s *fileService) GetFileForDownload(boardID string, wrID int, fileNo int) (*domain.FileDownloadInfo, error) {
	bf, err := s.repo.FindByID(boardID, wrID, fileNo)
	if err != nil {
		return nil, fmt.Errorf("파일을 찾을 수 없습니다")
	}

	// 다운로드 카운트 증가
	go s.repo.IncrementDownload(boardID, wrID, fileNo) //nolint:errcheck // 비동기

	filePath := filepath.Join(s.uploadPath, boardID, bf.File)
	contentType := detectContentType(filepath.Ext(bf.Source))

	return &domain.FileDownloadInfo{
		FilePath:    filePath,
		Source:      bf.Source,
		ContentType: contentType,
	}, nil
}

// saveFile saves an uploaded file to disk and creates DB record
func (s *fileService) saveFile(file *multipart.FileHeader, boardID string, wrID int, isImage bool) (*domain.FileUploadResponse, error) {
	// 다음 파일 번호
	fileNo, err := s.repo.GetNextFileNo(boardID, wrID)
	if err != nil {
		return nil, err
	}

	// 저장 디렉토리 생성
	yearMonth := time.Now().Format("200601")
	dirPath := filepath.Join(s.uploadPath, boardID, yearMonth)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return nil, fmt.Errorf("디렉토리 생성 실패: %w", err)
	}

	// 고유 파일명 생성
	ext := filepath.Ext(file.Filename)
	savedName := fmt.Sprintf("%d_%d_%d%s", time.Now().Unix(), wrID, fileNo, ext)
	savePath := filepath.Join(dirPath, savedName)

	// 파일 저장
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("파일 열기 실패: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(savePath)
	if err != nil {
		return nil, fmt.Errorf("파일 생성 실패: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return nil, fmt.Errorf("파일 저장 실패: %w", err)
	}

	// DB 저장
	fileType := 0
	if isImage {
		fileType = 1
	}

	relativeFile := filepath.Join(yearMonth, savedName)
	bf := &domain.BoardFile{
		BoardID:  boardID,
		WriteID:  wrID,
		FileNo:   fileNo,
		Source:   file.Filename,
		File:     relativeFile,
		FileSize: int(file.Size),
		Type:     fileType,
		DateTime: time.Now(),
	}

	if err := s.repo.Create(bf); err != nil {
		// DB 실패 시 파일 삭제
		os.Remove(savePath)
		return nil, err
	}

	// URL 생성
	url := fmt.Sprintf("/data/file/%s/%s", boardID, relativeFile)

	return &domain.FileUploadResponse{
		URL:      url,
		FileName: file.Filename,
		FileSize: file.Size,
	}, nil
}

// File extension constants
const (
	extJPG  = ".jpg"
	extJPEG = ".jpeg"
	extPNG  = ".png"
	extGIF  = ".gif"
	extWebP = ".webp"
	extPDF  = ".pdf"
	extZip  = ".zip"
	extTxt  = ".txt"
)

// detectContentType returns content type from file extension
func detectContentType(ext string) string {
	switch strings.ToLower(ext) {
	case extJPG, extJPEG:
		return "image/jpeg"
	case extPNG:
		return "image/png"
	case extGIF:
		return "image/gif"
	case extWebP:
		return "image/webp"
	case extPDF:
		return "application/pdf"
	case extZip:
		return "application/zip"
	case extTxt:
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}
