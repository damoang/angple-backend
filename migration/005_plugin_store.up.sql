-- 플러그인 설치/상태 관리
CREATE TABLE IF NOT EXISTS plugin_installations (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    plugin_name VARCHAR(100) NOT NULL,
    version VARCHAR(50) NOT NULL,
    status ENUM('enabled', 'disabled', 'error') NOT NULL DEFAULT 'disabled',
    installed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    enabled_at TIMESTAMP NULL,
    disabled_at TIMESTAMP NULL,
    config JSON NULL,
    error_message TEXT NULL,
    installed_by VARCHAR(100) NULL,
    UNIQUE KEY uk_plugin_name (plugin_name),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 플러그인 개별 설정 (key-value)
CREATE TABLE IF NOT EXISTS plugin_settings (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    plugin_name VARCHAR(100) NOT NULL,
    setting_key VARCHAR(200) NOT NULL,
    setting_value TEXT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_plugin_setting (plugin_name, setting_key),
    INDEX idx_plugin_name (plugin_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 플러그인 이벤트 감사 로그
CREATE TABLE IF NOT EXISTS plugin_events (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    plugin_name VARCHAR(100) NOT NULL,
    event_type ENUM('installed','enabled','disabled','uninstalled','config_changed','error') NOT NULL,
    details JSON NULL,
    actor_id VARCHAR(100) NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_plugin_event (plugin_name, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
