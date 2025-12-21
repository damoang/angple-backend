package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 애플리케이션 설정 구조체
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Redis     RedisConfig     `yaml:"redis"`
	JWT       JWTConfig       `yaml:"jwt"`
	DataPaths DataPathsConfig `yaml:"data_paths"`
	CORS      CORSConfig      `yaml:"cors"`
}

// DataPathsConfig 데이터 경로 설정
type DataPathsConfig struct {
	RecommendedPath string `yaml:"recommended_path"`
}

// ServerConfig 서버 설정
type ServerConfig struct {
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"` // development, staging, production
	Env  string `yaml:"env"`  // local, dev, staging, prod
}

// DatabaseConfig 데이터베이스 설정
type DatabaseConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	DBName          string `yaml:"dbname"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"` // seconds
}

// RedisConfig Redis 설정
type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	PoolSize int    `yaml:"pool_size"`
}

// JWTConfig JWT 설정
type JWTConfig struct {
	Secret        string `yaml:"secret"`
	ExpiresIn     int    `yaml:"expires_in"`      // seconds (access token)
	RefreshIn     int    `yaml:"refresh_in"`      // seconds (refresh token)
	DamoangSecret string `yaml:"damoang_secret"`  // damoang.net JWT secret
}

// CORSConfig CORS 설정
type CORSConfig struct {
	AllowOrigins string `yaml:"allow_origins"`  // Comma-separated list
}

// Load 설정 파일 로드
func Load(configPath string) (*Config, error) {
	// 설정 파일 읽기
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// YAML 파싱
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// 환경 변수로 오버라이드
	overrideFromEnv(&cfg)

	return &cfg, nil
}

// overrideFromEnv 환경 변수로 설정 오버라이드
func overrideFromEnv(cfg *Config) {
	// DB 설정
	if host := os.Getenv("DB_HOST"); host != "" {
		cfg.Database.Host = host
	}
	if port := os.Getenv("DB_PORT"); port != "" {
		fmt.Sscanf(port, "%d", &cfg.Database.Port)
	}
	if user := os.Getenv("DB_USER"); user != "" {
		cfg.Database.User = user
	}
	if password := os.Getenv("DB_PASSWORD"); password != "" {
		cfg.Database.Password = password
	}
	if dbname := os.Getenv("DB_NAME"); dbname != "" {
		cfg.Database.DBName = dbname
	}

	// Redis 설정
	if host := os.Getenv("REDIS_HOST"); host != "" {
		cfg.Redis.Host = host
	}
	if port := os.Getenv("REDIS_PORT"); port != "" {
		fmt.Sscanf(port, "%d", &cfg.Redis.Port)
	}

	// JWT 설정
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		cfg.JWT.Secret = secret
	}
	if damoangSecret := os.Getenv("DAMOANG_JWT_SECRET"); damoangSecret != "" {
		cfg.JWT.DamoangSecret = damoangSecret
	}

	// 서버 설정
	if port := os.Getenv("API_PORT"); port != "" {
		fmt.Sscanf(port, "%d", &cfg.Server.Port)
	}

	// 데이터 경로 설정
	if recommendedPath := os.Getenv("RECOMMENDED_DATA_PATH"); recommendedPath != "" {
		cfg.DataPaths.RecommendedPath = recommendedPath
	}

	// CORS 설정
	if corsOrigins := os.Getenv("CORS_ALLOW_ORIGINS"); corsOrigins != "" {
		cfg.CORS.AllowOrigins = corsOrigins
	}
}

// GetDSN MySQL DSN 문자열 생성
func (c *DatabaseConfig) GetDSN() string {
	// sql_mode='' 로 STRICT_TRANS_TABLES 비활성화 (NOT NULL 필드 기본값 허용)
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&sql_mode=''",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.DBName,
	)
}

// GetRedisAddr Redis 주소 문자열 생성
func (c *RedisConfig) GetRedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// IsDevelopment 개발 환경 여부 확인
func (c *Config) IsDevelopment() bool {
	return c.Server.Env == "local" || c.Server.Env == "dev"
}

// IsProduction 운영 환경 여부 확인
func (c *Config) IsProduction() bool {
	return c.Server.Env == "prod" || c.Server.Env == "production"
}
