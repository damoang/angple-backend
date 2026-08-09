-- 투표(Poll) 플러그인 테이블 (2026-08-09, 설계: /home/damoang/docs/poll-design.html)
-- ⛔ backend 는 auto-migrate 를 쓰지 않는다 — 배포 전 수동 실행 필수.
-- 시간 비교는 Go 에서만 한다(DSN loc=Asia/Seoul — SQL NOW() 와 섞으면 9시간 함정).

CREATE TABLE IF NOT EXISTS angple_polls (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    bo_table VARCHAR(20) NOT NULL,
    wr_id INT NOT NULL,
    mb_id VARCHAR(20) NOT NULL,
    question VARCHAR(200) NOT NULL DEFAULT '',
    allow_multi TINYINT NOT NULL DEFAULT 0,
    reveal ENUM('after_vote','after_close') NOT NULL DEFAULT 'after_vote',
    closes_at DATETIME NULL,
    status ENUM('open','closed') NOT NULL DEFAULT 'open',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_post (bo_table, wr_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS angple_poll_options (
    poll_id BIGINT UNSIGNED NOT NULL,
    idx TINYINT NOT NULL,
    label VARCHAR(100) NOT NULL,
    votes_count INT NOT NULL DEFAULT 0,
    PRIMARY KEY (poll_id, idx)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- PK(poll_id, mb_id, option_idx) 가 이중투표의 유일한 확실한 방어선.
CREATE TABLE IF NOT EXISTS angple_poll_votes (
    poll_id BIGINT UNSIGNED NOT NULL,
    mb_id VARCHAR(20) NOT NULL,
    option_idx TINYINT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (poll_id, mb_id, option_idx)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
