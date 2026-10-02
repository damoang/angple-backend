package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"strings"
	"testing"
	"time"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/damoang/angple-backend/pkg/storage"
)

var (
	testJPEG = append([]byte("\xFF\xD8\xFF\xE0"), bytes.Repeat([]byte{0}, 32)...)
	testPNG  = append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 32)...)
	testGIF  = append([]byte("GIF89a"), bytes.Repeat([]byte{0}, 32)...)
	testWEBP = append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), bytes.Repeat([]byte{0}, 32)...)
	testSVG  = []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"><script>alert(1)</script></svg>`)
)

// fakeMemberImageStore records uploads and answers Exists from a call budget.
type fakeMemberImageStore struct {
	basePath string
	uploads  []fakeUpload

	// existing: key -> 존재로 답하기 전까지 「없음」으로 답할 횟수. 없는 키는 항상 「없음」.
	existing    map[string]int
	existsCalls map[string]int
}

type fakeUpload struct {
	key         string
	body        []byte
	contentType string
	metadata    map[string]string
}

func newFakeMemberImageStore() *fakeMemberImageStore {
	return &fakeMemberImageStore{
		existing:    map[string]int{},
		existsCalls: map[string]int{},
	}
}

func (f *fakeMemberImageStore) UploadWithMetadata(_ context.Context, key string, body io.Reader, contentType string, size int64, metadata map[string]string) (*storage.UploadResult, error) {
	b, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	f.uploads = append(f.uploads, fakeUpload{key: key, body: b, contentType: contentType, metadata: metadata})
	fullKey := f.basePath + key
	return &storage.UploadResult{
		Key:         fullKey,
		URL:         "https://cdn.example.test/" + fullKey,
		CDNURL:      "https://cdn.example.test/" + fullKey,
		ContentType: contentType,
		Size:        size,
	}, nil
}

func (f *fakeMemberImageStore) Exists(_ context.Context, key string) (bool, error) {
	f.existsCalls[key]++
	misses, ok := f.existing[key]
	if !ok {
		return false, nil
	}
	return f.existsCalls[key] > misses, nil
}

// fakeMemberRepo only implements the image methods; other calls panic via the nil embed.
type fakeMemberRepo struct {
	gnurepo.MemberRepository
	updated map[string]string
	cleared []string
}

func (r *fakeMemberRepo) UpdateMemberImageUrl(mbID, imageURL string) error {
	if r.updated == nil {
		r.updated = map[string]string{}
	}
	r.updated[mbID] = imageURL
	return nil
}

func (r *fakeMemberRepo) ClearMemberImageUrl(mbID string) error {
	r.cleared = append(r.cleared, mbID)
	return nil
}

func newTestMemberService(store *fakeMemberImageStore, repo *fakeMemberRepo) *MemberService {
	return &MemberService{
		store:        store,
		memberRepo:   repo,
		now:          func() time.Time { return time.Unix(1700000000, 0) },
		pollInterval: time.Millisecond,
		pollTimeout:  50 * time.Millisecond,
	}
}

// TestNewMemberService_PollDefaults: 변환 대기 기본값(300ms 간격, 12초 상한)을 잠근다.
func TestNewMemberService_PollDefaults(t *testing.T) {
	svc := NewMemberService(nil, &fakeMemberRepo{})
	if svc.pollInterval != 300*time.Millisecond {
		t.Errorf("pollInterval = %v, want 300ms", svc.pollInterval)
	}
	if svc.pollTimeout != 12*time.Second {
		t.Errorf("pollTimeout = %v, want 12s", svc.pollTimeout)
	}
	if svc.store != nil {
		t.Error("S3 클라이언트가 nil 이면 store 도 nil 이어야 한다(typed nil 방지)")
	}
}

func TestMemberImageKeys(t *testing.T) {
	tests := []struct {
		name     string
		mbID     string
		wantRaw  string
		wantData string
		wantErr  bool
	}{
		{"일반 ID", "abcd", "raw/member_image/ab/abcd_100.jpg", "data/member_image/ab/abcd_100.webp", false},
		{"대소문자 유지", "AbCd", "raw/member_image/Ab/AbCd_100.jpg", "data/member_image/Ab/AbCd_100.webp", false},
		{"2글자 ID", "ab", "raw/member_image/ab/ab_100.jpg", "data/member_image/ab/ab_100.webp", false},
		{"1글자 ID 는 패닉 없이 그 글자", "a", "raw/member_image/a/a_100.jpg", "data/member_image/a/a_100.webp", false},
		{"멀티바이트는 글자 단위", "가나다", "raw/member_image/가나/가나다_100.jpg", "data/member_image/가나/가나다_100.webp", false},
		{"빈 ID 거부", "", "", "", true},
		{"슬래시 포함 거부", "a/b", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, data, err := memberImageKeys(tt.mbID, 100, ".jpg")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("에러를 기대했으나 nil (raw=%q)", raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("예상치 못한 에러: %v", err)
			}
			if raw != tt.wantRaw {
				t.Errorf("raw = %q, want %q", raw, tt.wantRaw)
			}
			if data != tt.wantData {
				t.Errorf("data = %q, want %q", data, tt.wantData)
			}
		})
	}
}

func TestValidateProfileImage(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		data     []byte
		wantExt  string
		wantCT   string
		wantErr  bool
	}{
		{"jpg", "a.jpg", testJPEG, ".jpg", "image/jpeg", false},
		{"jpeg 확장자 유지", "a.JPEG", testJPEG, ".jpeg", "image/jpeg", false},
		{"png", "a.png", testPNG, ".png", "image/png", false},
		{"gif", "a.gif", testGIF, ".gif", "image/gif", false},
		{"webp", "a.webp", testWEBP, ".webp", "image/webp", false},
		{"확장자와 실제 형식이 다르면 실제 형식", "a.png", testJPEG, ".jpg", "image/jpeg", false},
		{"webp 를 jpg 로 이름만 바꾼 경우", "a.jpg", testWEBP, ".webp", "image/webp", false},
		{"svg 확장자 거부", "a.svg", testSVG, "", "", true},
		{"png 로 이름만 바꾼 svg 거부", "a.png", testSVG, "", "", true},
		{"heic 확장자 거부", "a.heic", testJPEG, "", "", true},
		{"확장자 없음 거부", "image", testJPEG, "", "", true},
		{"이미지 아닌 내용 거부", "a.jpg", []byte("hello world, not an image"), "", "", true},
		{"빈 파일 거부", "a.jpg", nil, "", "", true},
		{"5MB 초과 거부", "a.jpg", append(append([]byte{}, testJPEG...), make([]byte, maxProfileImageSize)...), "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext, ct, err := validateProfileImage(tt.filename, tt.data)
			if tt.wantErr {
				var verr *MemberImageValidationError
				if !errors.As(err, &verr) {
					t.Fatalf("MemberImageValidationError 를 기대했으나 %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("예상치 못한 에러: %v", err)
			}
			if ext != tt.wantExt {
				t.Errorf("ext = %q, want %q", ext, tt.wantExt)
			}
			if ct != tt.wantCT {
				t.Errorf("content type = %q, want %q", ct, tt.wantCT)
			}
		})
	}
}

func TestUploadMemberImage_RawUploadThenStoresWebpKey(t *testing.T) {
	store := newFakeMemberImageStore()
	store.existing["data/member_image/Ab/AbCd_1700000000.webp"] = 2 // 세 번째 확인에서 생김
	repo := &fakeMemberRepo{}
	svc := newTestMemberService(store, repo)

	url, err := svc.uploadMemberImage(context.Background(), "AbCd", "photo.png", testPNG)
	if err != nil {
		t.Fatalf("업로드 실패: %v", err)
	}

	if len(store.uploads) != 1 {
		t.Fatalf("업로드 횟수 = %d, want 1", len(store.uploads))
	}
	up := store.uploads[0]
	if up.key != "raw/member_image/Ab/AbCd_1700000000.png" {
		t.Errorf("raw key = %q", up.key)
	}
	if up.metadata["Photo-Type"] != "member-profile" {
		t.Errorf("metadata = %v, Photo-Type=member-profile 이 있어야 한다", up.metadata)
	}
	if up.contentType != "image/png" {
		t.Errorf("content type = %q", up.contentType)
	}
	if !bytes.Equal(up.body, testPNG) {
		t.Error("원본 바이트가 그대로 올라가야 한다(서버 축소·재인코딩 없음)")
	}

	wantKey := "data/member_image/Ab/AbCd_1700000000.webp"
	if got := repo.updated["AbCd"]; got != wantKey {
		t.Errorf("DB mb_image_url = %q, want %q", got, wantKey)
	}
	if url != "https://cdn.example.test/"+wantKey {
		t.Errorf("url = %q", url)
	}
}

func TestUploadMemberImage_KeepsBasePathInStoredKey(t *testing.T) {
	store := newFakeMemberImageStore()
	store.basePath = "base/"
	store.existing["data/member_image/ab/abc_1700000000.webp"] = 0
	repo := &fakeMemberRepo{}
	svc := newTestMemberService(store, repo)

	url, err := svc.uploadMemberImage(context.Background(), "abc", "a.jpg", testJPEG)
	if err != nil {
		t.Fatalf("업로드 실패: %v", err)
	}
	wantKey := "base/data/member_image/ab/abc_1700000000.webp"
	if got := repo.updated["abc"]; got != wantKey {
		t.Errorf("DB mb_image_url = %q, want %q", got, wantKey)
	}
	if url != "https://cdn.example.test/"+wantKey {
		t.Errorf("url = %q", url)
	}
}

// TestUploadMemberImage_TimeoutLeavesDBUnchanged: 변환 결과가 시간 안에 안 생기면
// DB 를 바꾸지 않고 타임아웃 에러를 돌려준다.
func TestUploadMemberImage_TimeoutLeavesDBUnchanged(t *testing.T) {
	store := newFakeMemberImageStore()
	repo := &fakeMemberRepo{}
	svc := newTestMemberService(store, repo)

	_, err := svc.uploadMemberImage(context.Background(), "abcd", "a.jpg", testJPEG)
	if !errors.Is(err, ErrMemberImageProcessingTimeout) {
		t.Fatalf("err = %v, want ErrMemberImageProcessingTimeout", err)
	}
	if len(repo.updated) != 0 {
		t.Errorf("타임아웃인데 DB 가 바뀌었다: %v", repo.updated)
	}
	if len(store.uploads) != 1 {
		t.Errorf("원본은 한 번 올라가야 한다: %d", len(store.uploads))
	}
}

func TestUploadMemberImage_RejectsSVGWithoutUpload(t *testing.T) {
	for _, name := range []string{"a.svg", "a.png"} {
		store := newFakeMemberImageStore()
		repo := &fakeMemberRepo{}
		svc := newTestMemberService(store, repo)

		_, err := svc.uploadMemberImage(context.Background(), "abcd", name, testSVG)
		var verr *MemberImageValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("%s: SVG 가 거부되지 않았다: %v", name, err)
		}
		if len(store.uploads) != 0 || len(repo.updated) != 0 {
			t.Errorf("%s: 거부된 파일이 업로드·저장되었다", name)
		}
	}
}

// TestUploadMemberImage_SameSecondDoesNotOverwrite: 같은 초에 다시 올려도 이전 원본을 덮지 않는다.
func TestUploadMemberImage_SameSecondDoesNotOverwrite(t *testing.T) {
	store := newFakeMemberImageStore()
	store.existing["raw/member_image/ab/abcd_1700000000.jpg"] = 0
	store.existing["data/member_image/ab/abcd_1700000001.webp"] = 0
	repo := &fakeMemberRepo{}
	svc := newTestMemberService(store, repo)

	if _, err := svc.uploadMemberImage(context.Background(), "abcd", "a.jpg", testJPEG); err != nil {
		t.Fatalf("업로드 실패: %v", err)
	}
	if got := store.uploads[0].key; got != "raw/member_image/ab/abcd_1700000001.jpg" {
		t.Errorf("raw key = %q, 겹치지 않는 다음 시각이어야 한다", got)
	}
	if got := repo.updated["abcd"]; got != "data/member_image/ab/abcd_1700000001.webp" {
		t.Errorf("DB mb_image_url = %q", got)
	}
}

func TestUpdateMemberImage_RejectsOversizeBeforeReading(t *testing.T) {
	svc := newTestMemberService(newFakeMemberImageStore(), &fakeMemberRepo{})
	// 내용 없는 헤더: 크기 검사가 Open 보다 먼저여야 패닉·에러 없이 거부된다.
	fh := &multipart.FileHeader{Filename: "a.jpg", Size: maxProfileImageSize + 1}

	_, err := svc.UpdateMemberImage(context.Background(), "abcd", fh)
	var verr *MemberImageValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want MemberImageValidationError", err)
	}
	if !strings.Contains(verr.Msg, "5MB") {
		t.Errorf("문구에 상한이 있어야 한다: %q", verr.Msg)
	}
}

// TestDeleteMemberImage_KeepsStorage: 삭제는 DB 만 비우고 저장소 객체는 건드리지 않는다.
func TestDeleteMemberImage_KeepsStorage(t *testing.T) {
	store := newFakeMemberImageStore()
	repo := &fakeMemberRepo{}
	svc := newTestMemberService(store, repo)

	if err := svc.DeleteMemberImage(context.Background(), "abcd"); err != nil {
		t.Fatalf("삭제 실패: %v", err)
	}
	if len(repo.cleared) != 1 || repo.cleared[0] != "abcd" {
		t.Errorf("cleared = %v", repo.cleared)
	}
	if len(store.uploads) != 0 || len(store.existsCalls) != 0 {
		t.Error("삭제가 저장소를 건드렸다")
	}
}
