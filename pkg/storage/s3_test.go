package storage

import (
	"context"
	"path/filepath"
	"testing"
)

// isolateAWSEnv keeps the default credential chain from reading the host's
// files, environment or instance metadata.
func isolateAWSEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(dir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(dir, "credentials"))
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
}

// TestNewAWSS3Client_StaticKeys: 키가 둘 다 있으면 기존처럼 정적 자격증명을 쓴다.
func TestNewAWSS3Client_StaticKeys(t *testing.T) {
	isolateAWSEnv(t)
	// 기본 체인이 쓰이면 이 값이 나온다. 정적 키가 이겨야 한다.
	t.Setenv("AWS_ACCESS_KEY_ID", "env-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "env-secret")

	client, err := newAWSS3Client(context.Background(), S3Config{
		Endpoint:        "https://storage.example.test",
		Region:          "auto",
		AccessKeyID:     "static-key",
		SecretAccessKey: "static-secret",
		ForcePathStyle:  true,
	})
	if err != nil {
		t.Fatalf("클라이언트 생성 실패: %v", err)
	}

	opts := client.Options()
	creds, err := opts.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("자격증명 조회 실패: %v", err)
	}
	if creds.AccessKeyID != "static-key" {
		t.Errorf("AccessKeyID = %q, want static-key", creds.AccessKeyID)
	}
	if opts.Region != "auto" {
		t.Errorf("Region = %q, want auto", opts.Region)
	}
	if opts.BaseEndpoint == nil || *opts.BaseEndpoint != "https://storage.example.test" {
		t.Errorf("BaseEndpoint = %v", opts.BaseEndpoint)
	}
	if !opts.UsePathStyle {
		t.Error("UsePathStyle 이 유지되어야 한다")
	}
}

// TestNewAWSS3Client_EmptyKeysUseDefaultChain: 키가 비어 있으면 빈 정적 자격증명이 아니라
// SDK 기본 체인을 쓴다. 여기서는 체인의 첫 단계(환경변수)로 확인한다.
func TestNewAWSS3Client_EmptyKeysUseDefaultChain(t *testing.T) {
	isolateAWSEnv(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "env-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "env-secret")

	client, err := newAWSS3Client(context.Background(), S3Config{
		Endpoint: "https://s3.ap-northeast-2.amazonaws.com",
		Region:   "ap-northeast-2",
	})
	if err != nil {
		t.Fatalf("클라이언트 생성 실패: %v", err)
	}

	opts := client.Options()
	if opts.Credentials == nil {
		t.Fatal("자격증명 공급자가 없다")
	}
	creds, err := opts.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("자격증명 조회 실패: %v", err)
	}
	if creds.AccessKeyID != "env-key" {
		t.Errorf("AccessKeyID = %q, want env-key (기본 체인)", creds.AccessKeyID)
	}
	if opts.Region != "ap-northeast-2" {
		t.Errorf("Region = %q, want ap-northeast-2", opts.Region)
	}
	if opts.BaseEndpoint == nil || *opts.BaseEndpoint != "https://s3.ap-northeast-2.amazonaws.com" {
		t.Errorf("BaseEndpoint = %v", opts.BaseEndpoint)
	}
	if opts.UsePathStyle {
		t.Error("UsePathStyle 은 설정값(false)을 따라야 한다")
	}
}

// TestNewAWSS3Client_PartialKeysRejected: 키가 하나만 있으면 기본 체인으로 넘어가지 않고 에러다.
func TestNewAWSS3Client_PartialKeysRejected(t *testing.T) {
	isolateAWSEnv(t)

	cases := []S3Config{
		{Region: "ap-northeast-2", AccessKeyID: "only-id"},
		{Region: "ap-northeast-2", SecretAccessKey: "only-secret"},
	}
	for _, cfg := range cases {
		if _, err := newAWSS3Client(context.Background(), cfg); err == nil {
			t.Errorf("키가 하나만 있는 설정이 거부되지 않았다: id=%q", cfg.AccessKeyID)
		}
	}
}

func TestHasStaticCredentials(t *testing.T) {
	if hasStaticCredentials(S3Config{}) {
		t.Error("빈 키는 정적 자격증명이 아니다")
	}
	if !hasStaticCredentials(S3Config{AccessKeyID: "a", SecretAccessKey: "b"}) {
		t.Error("두 키가 있으면 정적 자격증명이다")
	}
}
