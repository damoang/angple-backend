# 그누보드 5 ↔ Angple Backend 스키마 매핑 가이드

## 개요

Angple Backend는 그누보드 5의 데이터베이스 스키마와 100% 호환됩니다.
이 문서는 그누보드 테이블/컬럼과 Angple의 Go 구조체 간 매핑 관계를 정의합니다.

---

## 1. 핵심 테이블 매핑

### 1.1 회원 (g5_member)

**그누보드 테이블:** `g5_member`

**Go 구조체:** `internal/domain/member.go`

```go
type Member struct {
    MemberNo     int       `gorm:"column:mb_no;primaryKey;autoIncrement" json:"member_no"`
    ID           string    `gorm:"column:mb_id;unique;size:20" json:"id"`
    Password     string    `gorm:"column:mb_password;size:255" json:"-"`
    Name         string    `gorm:"column:mb_name;size:255" json:"name"`
    Nick         string    `gorm:"column:mb_nick;size:255" json:"nick"`
    Email        string    `gorm:"column:mb_email;size:255" json:"email"`
    Level        int       `gorm:"column:mb_level;default:1" json:"level"`
    Tel          string    `gorm:"column:mb_tel;size:20" json:"tel"`
    HP           string    `gorm:"column:mb_hp;size:20" json:"hp"`
    Point        int       `gorm:"column:mb_point;default:0" json:"point"`
    DateTime     time.Time `gorm:"column:mb_datetime" json:"datetime"`
    LeaveDate    string    `gorm:"column:mb_leave_date;size:8" json:"leave_date,omitempty"`
    IP           string    `gorm:"column:mb_ip;size:255" json:"ip,omitempty"`
}

func (Member) TableName() string {
    return "g5_member"
}
```

**주요 필드 설명:**
- `mb_no`: 일련번호 (AUTO_INCREMENT, PRIMARY KEY)
- `mb_id`: 회원 ID (UNIQUE, 로그인용)
- `mb_password`: 비밀번호 (3가지 해싱 방식 지원 - `pkg/auth/legacy.go` 참고)
- `mb_level`: 권한 레벨 (1~10, 관리자는 10)
- `mb_point`: 보유 포인트
- `mb_datetime`: 가입일시
- `mb_leave_date`: 탈퇴일 (YYYYMMDD 형식)

**비밀번호 검증:**
```go
// pkg/auth/legacy.go
func VerifyGnuboardPassword(plainPassword, hashedPassword string) bool {
    // 1. MySQL PASSWORD() - *접두사 + 40자
    // 2. SHA1 - 40자
    // 3. 평문 (오래된 계정)
}
```

---

### 1.2 게시판 설정 (g5_board)

**그누보드 테이블:** `g5_board`

**Go 구조체:** `internal/domain/board.go`

```go
type Board struct {
    BoardID         string `gorm:"column:bo_table;primaryKey;size:20" json:"board_id"`
    Subject         string `gorm:"column:bo_subject;size:255" json:"subject"`
    GroupID         string `gorm:"column:gr_id;size:20" json:"group_id"`
    Admin           string `gorm:"column:bo_admin;size:255" json:"admin"`

    // 권한 레벨
    ListLevel       int    `gorm:"column:bo_list_level;default:1" json:"list_level"`
    ReadLevel       int    `gorm:"column:bo_read_level;default:1" json:"read_level"`
    WriteLevel      int    `gorm:"column:bo_write_level;default:1" json:"write_level"`
    ReplyLevel      int    `gorm:"column:bo_reply_level;default:1" json:"reply_level"`
    CommentLevel    int    `gorm:"column:bo_comment_level;default:1" json:"comment_level"`

    // 게시판 옵션
    UseCategory     int    `gorm:"column:bo_use_category;default:0" json:"use_category"`
    CategoryList    string `gorm:"column:bo_category_list" json:"category_list"`
    Skin            string `gorm:"column:bo_skin;size:255" json:"skin"`
    MobileSkin      string `gorm:"column:bo_mobile_skin;size:255" json:"mobile_skin"`
    PageRows        int    `gorm:"column:bo_page_rows;default:15" json:"page_rows"`

    // 첨부파일 설정
    UploadCount     int    `gorm:"column:bo_upload_count;default:2" json:"upload_count"`
    UploadSize      int    `gorm:"column:bo_upload_size;default:1048576" json:"upload_size"` // 1MB

    // 여분 필드 (확장용)
    Extra1          string `gorm:"column:bo_1;size:255" json:"extra_1,omitempty"`
    Extra2          string `gorm:"column:bo_2;size:255" json:"extra_2,omitempty"`
    Extra3          string `gorm:"column:bo_3;size:255" json:"extra_3,omitempty"`
    Extra4          string `gorm:"column:bo_4;size:255" json:"extra_4,omitempty"`
    Extra5          string `gorm:"column:bo_5;size:255" json:"extra_5,omitempty"`
}

func (Board) TableName() string {
    return "g5_board"
}
```

**주요 필드 설명:**
- `bo_table`: 게시판 ID (PRIMARY KEY, 예: "free", "notice")
- `bo_subject`: 게시판 이름
- `bo_*_level`: 권한 레벨 (1~10)
  - 1: 비회원
  - 2~9: 일반 회원
  - 10: 관리자
- `bo_use_category`: 카테고리 사용 여부 (0: 미사용, 1: 사용)
- `bo_category_list`: 카테고리 목록 (파이프 구분, 예: "공지|자유|질문")

---

### 1.3 게시글/댓글 (g5_write_*)

**그누보드 테이블:** `g5_write_{board_id}` (동적 생성)

**Go 구조체:** `internal/domain/post.go`

```go
type Post struct {
    ID            int       `gorm:"column:wr_id;primaryKey;autoIncrement" json:"id"`
    Num           int       `gorm:"column:wr_num" json:"num"`
    Reply         string    `gorm:"column:wr_reply;size:10" json:"reply"`
    Parent        int       `gorm:"column:wr_parent;default:0" json:"parent"`
    IsComment     int       `gorm:"column:wr_is_comment;default:0" json:"is_comment"`

    // 콘텐츠
    Subject       string    `gorm:"column:wr_subject;size:255" json:"subject"`
    Content       string    `gorm:"column:wr_content;type:text" json:"content"`
    SEOTitle      string    `gorm:"column:wr_seo_title;size:255" json:"seo_title,omitempty"`
    Category      string    `gorm:"column:ca_name;size:255" json:"category,omitempty"`

    // 작성자 정보
    MemberID      string    `gorm:"column:mb_id;size:20" json:"member_id"`
    Name          string    `gorm:"column:wr_name;size:255" json:"name"`
    Password      string    `gorm:"column:wr_password;size:255" json:"-"` // 비회원용
    Email         string    `gorm:"column:wr_email;size:255" json:"email,omitempty"`
    Homepage      string    `gorm:"column:wr_homepage;size:255" json:"homepage,omitempty"`

    // 메타데이터
    DateTime      time.Time `gorm:"column:wr_datetime" json:"datetime"`
    Last          string    `gorm:"column:wr_last;size:19" json:"last"`
    IP            string    `gorm:"column:wr_ip;size:255" json:"ip"`
    Hit           int       `gorm:"column:wr_hit;default:0" json:"hit"`
    Good          int       `gorm:"column:wr_good;default:0" json:"good"`
    NoGood        int       `gorm:"column:wr_nogood;default:0" json:"nogood"`

    // 첨부파일
    FileCount     int       `gorm:"column:wr_file;default:0" json:"file_count"`

    // 링크
    Link1         string    `gorm:"column:wr_link1;type:text" json:"link1,omitempty"`
    Link2         string    `gorm:"column:wr_link2;type:text" json:"link2,omitempty"`
    Link1Hit      int       `gorm:"column:wr_link1_hit;default:0" json:"link1_hit"`
    Link2Hit      int       `gorm:"column:wr_link2_hit;default:0" json:"link2_hit"`

    // 여분 필드 (확장용, 10개)
    Extra1        string    `gorm:"column:wr_1;size:255" json:"extra_1,omitempty"`
    Extra2        string    `gorm:"column:wr_2;size:255" json:"extra_2,omitempty"`
    Extra3        string    `gorm:"column:wr_3;size:255" json:"extra_3,omitempty"`
    Extra4        string    `gorm:"column:wr_4;size:255" json:"extra_4,omitempty"`
    Extra5        string    `gorm:"column:wr_5;size:255" json:"extra_5,omitempty"`
    Extra6        string    `gorm:"column:wr_6;size:255" json:"extra_6,omitempty"`
    Extra7        string    `gorm:"column:wr_7;size:255" json:"extra_7,omitempty"`
    Extra8        string    `gorm:"column:wr_8;size:255" json:"extra_8,omitempty"`
    Extra9        string    `gorm:"column:wr_9;size:255" json:"extra_9,omitempty"`
    Extra10       string    `gorm:"column:wr_10;size:255" json:"extra_10,omitempty"`
}

// 동적 테이블 지정 (Repository에서 사용)
// TableName()을 사용하지 않고 db.Table()로 동적 지정
```

**핵심 개념:**

#### 1) 게시글과 댓글이 같은 테이블에 저장됨

```go
// 게시글
wr_is_comment = 0

// 댓글
wr_is_comment = 1
wr_parent = {게시글 wr_id}
```

#### 2) 동적 테이블 처리

```go
// Repository에서 동적으로 테이블 지정
func (r *PostRepository) FindByID(boardID string, postID int) (*domain.Post, error) {
    tableName := fmt.Sprintf("g5_write_%s", boardID)

    var post domain.Post
    err := r.db.Table(tableName).
        Where("wr_id = ? AND wr_is_comment = 0", postID).
        First(&post).Error

    return &post, err
}
```

#### 3) 계층형 답글 구조

`wr_reply` 필드로 답글 계층 표현:

```
원글:     wr_reply = ""
답글1:    wr_reply = "a"
답글1-1:  wr_reply = "aa"
답글1-2:  wr_reply = "ab"
답글2:    wr_reply = "b"
```

---

### 1.4 첨부파일 (g5_board_file)

**그누보드 테이블:** `g5_board_file`

**Go 구조체:** `internal/domain/file.go`

```go
type BoardFile struct {
    BoardID      string    `gorm:"column:bo_table;primaryKey;size:20" json:"board_id"`
    WriteID      int       `gorm:"column:wr_id;primaryKey" json:"write_id"`
    FileNo       int       `gorm:"column:bf_no;primaryKey" json:"file_no"`

    Source       string    `gorm:"column:bf_source;size:255" json:"source"`       // 원본 파일명
    File         string    `gorm:"column:bf_file;size:255" json:"file"`           // 저장된 파일명
    Download     int       `gorm:"column:bf_download;default:0" json:"download"`  // 다운로드 횟수
    Content      string    `gorm:"column:bf_content;type:text" json:"content"`    // 파일 설명

    FileSize     int       `gorm:"column:bf_filesize" json:"file_size"`
    Width        int       `gorm:"column:bf_width;default:0" json:"width"`        // 이미지 너비
    Height       int       `gorm:"column:bf_height;default:0" json:"height"`      // 이미지 높이
    Type         int       `gorm:"column:bf_type;default:0" json:"type"`          // 0: 일반, 1: 이미지, 2: 플래시
    DateTime     time.Time `gorm:"column:bf_datetime" json:"datetime"`
}

func (BoardFile) TableName() string {
    return "g5_board_file"
}
```

**복합키:**
- PRIMARY KEY (`bo_table`, `wr_id`, `bf_no`)

**파일 경로 규칙:**
```
data/file/{board_id}/{YYYYMM}/{bf_file}

예: data/file/free/202601/1704629123_abc123.jpg
```

---

## 2. Repository 패턴

### 2.1 동적 테이블 처리

```go
// internal/repository/post_repo.go
type PostRepository struct {
    db *gorm.DB
}

func (r *PostRepository) FindByID(boardID string, postID int) (*domain.Post, error) {
    tableName := fmt.Sprintf("g5_write_%s", boardID)

    var post domain.Post
    err := r.db.Table(tableName).
        Where("wr_id = ? AND wr_is_comment = 0", postID).
        First(&post).Error

    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, common.ErrNotFound
        }
        return nil, err
    }

    return &post, nil
}
```

### 2.2 댓글 조회

```go
func (r *PostRepository) FindComments(boardID string, postID int) ([]domain.Post, error) {
    tableName := fmt.Sprintf("g5_write_%s", boardID)

    var comments []domain.Post
    err := r.db.Table(tableName).
        Where("wr_parent = ? AND wr_is_comment = 1", postID).
        Order("wr_id ASC").
        Find(&comments).Error

    return comments, err
}
```

### 2.3 게시글 작성 (트랜잭션)

```go
func (r *PostRepository) Create(boardID string, post *domain.Post) error {
    tableName := fmt.Sprintf("g5_write_%s", boardID)

    return r.db.Transaction(func(tx *gorm.DB) error {
        // 1. 게시글 삽입
        if err := tx.Table(tableName).Create(post).Error; err != nil {
            return err
        }

        // 2. wr_num 계산 (정렬용 번호)
        post.Num = -post.ID
        if err := tx.Table(tableName).
            Where("wr_id = ?", post.ID).
            Update("wr_num", post.Num).Error; err != nil {
            return err
        }

        // 3. g5_board_new 테이블에 신규 글 기록
        if err := r.createBoardNew(tx, boardID, post); err != nil {
            return err
        }

        return nil
    })
}
```

---

## 3. Service 레이어 패턴

### 3.1 권한 검증

```go
// internal/service/post_service.go
func (s *PostService) UpdatePost(boardID string, postID int, userID string, req *domain.UpdatePostRequest) error {
    // 1. 게시글 조회
    post, err := s.repo.FindByID(boardID, postID)
    if err != nil {
        return err
    }

    // 2. 소유자 확인
    if post.MemberID != userID {
        return common.ErrForbidden
    }

    // 3. 업데이트 수행
    return s.repo.Update(boardID, postID, req)
}
```

### 3.2 레벨 기반 권한 검증

```go
func (s *BoardService) CanWrite(boardID string, memberLevel int) (bool, error) {
    board, err := s.repo.FindByID(boardID)
    if err != nil {
        return false, err
    }

    return memberLevel >= board.WriteLevel, nil
}
```

---

## 4. 마이그레이션 SQL

### 4.1 게시판 동적 테이블 생성

```sql
-- /adm/sql_write.sql 템플릿 기반
CREATE TABLE IF NOT EXISTS `g5_write_{board_id}` (
  `wr_id` int(11) NOT NULL AUTO_INCREMENT,
  `wr_num` int(11) NOT NULL DEFAULT 0,
  `wr_reply` varchar(10) NOT NULL,
  `wr_parent` int(11) NOT NULL DEFAULT 0,
  `wr_is_comment` tinyint(4) NOT NULL DEFAULT 0,
  `wr_comment` int(11) NOT NULL DEFAULT 0,
  `ca_name` varchar(255) NOT NULL,
  `wr_option` set('html1','html2','secret','mail') NOT NULL,
  `wr_subject` varchar(255) NOT NULL,
  `wr_content` text NOT NULL,
  `wr_seo_title` varchar(255) NOT NULL DEFAULT '',
  `wr_link1` text NOT NULL,
  `wr_link2` text NOT NULL,
  `wr_link1_hit` int(11) NOT NULL DEFAULT 0,
  `wr_link2_hit` int(11) NOT NULL DEFAULT 0,
  `wr_hit` int(11) NOT NULL DEFAULT 0,
  `wr_good` int(11) NOT NULL DEFAULT 0,
  `wr_nogood` int(11) NOT NULL DEFAULT 0,
  `mb_id` varchar(20) NOT NULL,
  `wr_password` varchar(255) NOT NULL,
  `wr_name` varchar(255) NOT NULL,
  `wr_email` varchar(255) NOT NULL,
  `wr_homepage` varchar(255) NOT NULL,
  `wr_datetime` datetime NOT NULL DEFAULT '0000-00-00 00:00:00',
  `wr_file` tinyint(4) NOT NULL DEFAULT 0,
  `wr_last` varchar(19) NOT NULL,
  `wr_ip` varchar(255) NOT NULL,
  `wr_facebook_user` varchar(255) NOT NULL,
  `wr_twitter_user` varchar(255) NOT NULL,
  `wr_1` varchar(255) NOT NULL,
  `wr_2` varchar(255) NOT NULL,
  `wr_3` varchar(255) NOT NULL,
  `wr_4` varchar(255) NOT NULL,
  `wr_5` varchar(255) NOT NULL,
  `wr_6` varchar(255) NOT NULL,
  `wr_7` varchar(255) NOT NULL,
  `wr_8` varchar(255) NOT NULL,
  `wr_9` varchar(255) NOT NULL,
  `wr_10` varchar(255) NOT NULL,
  PRIMARY KEY (`wr_id`),
  KEY `wr_num_reply_parent` (`wr_num`,`wr_reply`,`wr_parent`),
  KEY `wr_is_comment` (`wr_is_comment`,`wr_id`)
) ENGINE=MyISAM DEFAULT CHARSET=utf8;
```

---

## 5. API 엔드포인트 예시

### 5.1 게시판 관리

```
POST   /api/v2/boards                     # 게시판 생성 (관리자)
GET    /api/v2/boards                     # 게시판 목록
GET    /api/v2/boards/:board_id           # 게시판 정보
PUT    /api/v2/boards/:board_id           # 게시판 수정 (관리자)
DELETE /api/v2/boards/:board_id           # 게시판 삭제 (관리자)
```

### 5.2 게시글 관리

```
POST   /api/v2/boards/:board_id/posts                    # 게시글 작성
GET    /api/v2/boards/:board_id/posts                    # 게시글 목록
GET    /api/v2/boards/:board_id/posts/:post_id           # 게시글 조회
PUT    /api/v2/boards/:board_id/posts/:post_id           # 게시글 수정
DELETE /api/v2/boards/:board_id/posts/:post_id           # 게시글 삭제
POST   /api/v2/boards/:board_id/posts/:post_id/like      # 추천
POST   /api/v2/boards/:board_id/posts/:post_id/dislike   # 비추천
```

### 5.3 댓글 관리

```
POST   /api/v2/boards/:board_id/posts/:post_id/comments          # 댓글 작성
GET    /api/v2/boards/:board_id/posts/:post_id/comments          # 댓글 목록
PUT    /api/v2/boards/:board_id/posts/:post_id/comments/:cmt_id  # 댓글 수정
DELETE /api/v2/boards/:board_id/posts/:post_id/comments/:cmt_id  # 댓글 삭제
```

### 5.4 파일 관리

```
POST   /api/v2/boards/:board_id/posts/:post_id/files     # 파일 업로드
GET    /api/v2/boards/:board_id/posts/:post_id/files     # 파일 목록
GET    /api/v2/files/:file_id/download                   # 파일 다운로드
DELETE /api/v2/boards/:board_id/posts/:post_id/files/:file_no  # 파일 삭제
```

---

## 6. 주요 차이점 및 주의사항

### 6.1 GORM sql_mode 설정

그누보드는 MySQL STRICT 모드를 사용하지 않으므로:

```go
// cmd/api/main.go
dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&sql_mode=''",
    cfg.Database.User,
    cfg.Database.Password,
    cfg.Database.Host,
    cfg.Database.Port,
    cfg.Database.Name,
)
```

### 6.2 비밀번호 해싱 호환

그누보드는 3가지 방식 지원 (`pkg/auth/legacy.go` 참고):

```go
func VerifyGnuboardPassword(plainPassword, hashedPassword string) bool {
    // 1. MySQL PASSWORD() - *접두사 + SHA1(SHA1(password))
    if strings.HasPrefix(hashedPassword, "*") && len(hashedPassword) == 41 {
        return mysqlPassword(plainPassword) == hashedPassword
    }

    // 2. SHA1
    if len(hashedPassword) == 40 {
        return sha1Hash(plainPassword) == hashedPassword
    }

    // 3. 평문 (레거시)
    return plainPassword == hashedPassword
}
```

### 6.3 DateTime 필드 처리

그누보드는 `0000-00-00 00:00:00` 값을 사용하므로:

```go
// GORM 설정에서 parseTime=True 필수
// 빈 날짜는 Go의 zero time으로 변환됨
```

### 6.4 여분 필드 활용

확장 시 `wr_1` ~ `wr_10`, `mb_1` ~ `mb_10`, `bo_1` ~ `bo_5` 사용:

```go
// 예: 게시글에 태그 기능 추가
post.Extra1 = "golang,backend,api"  // wr_1에 저장
```

---

## 7. 참고 자료

- 그누보드 5 공식 GitHub: https://github.com/gnuboard/gnuboard5
- 스키마 SQL: https://github.com/gnuboard/gnuboard5/blob/master/install/gnuboard5.sql
- 게시판 테이블 템플릿: https://github.com/gnuboard/gnuboard5/blob/master/adm/sql_write.sql
- Angple Backend README: `/README.md`
- Angple CLAUDE.md: `/CLAUDE.md`
