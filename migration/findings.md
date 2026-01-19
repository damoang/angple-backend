# 발견 사항 및 참고 정보

## PHP 검증 로직 분석 (ang-gnu/lib/register.lib.php)

### ID 검증
```php
// 빈값 체크
if (trim($reg_mb_id)=='') return "회원아이디를 입력해 주십시오.";

// 형식 체크 (영문, 숫자, _ 만 허용)
if (preg_match("/[^0-9a-z_]+/i", $reg_mb_id)) return "회원아이디는 영문자, 숫자, _ 만 입력하세요.";

// 최소 길이
if (strlen($reg_mb_id) < 3) return "회원아이디는 최소 3글자 이상 입력하세요.";

// 중복 체크
SELECT count(*) from g5_member where mb_id = ?

// 예약어 체크
$config['cf_prohibit_id'] 에서 쉼표로 구분된 예약어 확인
```

### 닉네임 검증
```php
// 형식: 한글, 영문, 숫자, 연속되지 않은 ._ 허용
preg_match("/^(?!.*\.{2})(?!.*_{2})(?!.*[^a-zA-Z0-9가-힣ㄱ-ㅎㅏ-ㅣ_\.])[a-zA-Z0-9가-힣ㄱ-ㅎㅏ-ㅣ_\.]+/", $reg_mb_nick)

// 최소 길이: 4바이트 (한글 2글자 = 6바이트이므로 실제로는 영문 4글자 기준)
if (strlen($reg_mb_nick) < 4) return "닉네임은 한글 2글자, 영문 4글자 이상...";

// 중복 체크 (자기 자신 제외)
SELECT count(*) from g5_member where mb_nick = ? and mb_id <> ?
```

### 이메일 검증
```php
// 형식
preg_match("/([0-9a-zA-Z_-]+)@([0-9a-zA-Z_-]+)\.([0-9a-zA-Z_-]+)/", $reg_mb_email)

// 금지 도메인
$config['cf_prohibit_email'] 에서 줄바꿈으로 구분된 도메인 확인

// 중복 체크 (자기 자신 제외)
SELECT count(*) from g5_member where mb_email = ? and mb_id <> ?
```

### 휴대폰 검증
```php
// 숫자만 추출
$reg_mb_hp = preg_replace("/[^0-9]/", "", $reg_mb_hp);

// 형식: 01X로 시작, 10-11자리
preg_match("/^01[0-9]{8,9}$/", $reg_mb_hp)

// 중복 체크 (하이픈 형식으로 저장됨)
hyphen_hp_number($reg_mb_hp) // 010-1234-5678 형식으로 변환
SELECT count(*) from g5_member where mb_hp = ? and mb_id <> ?
```

---

## angple-backend 기존 구조

### Member 테이블 컬럼 (domain/member.go)
- `mb_id` (UserID) - 회원 ID
- `mb_nick` (Nickname) - 닉네임
- `mb_email` (Email) - 이메일
- `mb_hp` (Phone) - 휴대폰

### 기존 Repository 메서드 (repository/member_repo.go)
- `FindByUserID(userID string)` ✅
- `FindByEmail(email string)` ✅
- `ExistsByUserID(userID string)` ✅
- `ExistsByEmail(email string)` ✅
- `ExistsByNickname` ❌ 추가 필요
- `ExistsByPhone` ❌ 추가 필요
- `ExistsByEmailExcluding` ❌ 추가 필요

### 응답 형식 (common/response.go)
```go
// 성공
common.SuccessResponse(c, data, meta)

// 에러
common.ErrorResponse(c, status, message, err)
```

---

## 설정 테이블 (g5_config)
예약어와 금지 도메인 설정은 g5_config 테이블에 저장됨:
- `cf_prohibit_id` - 예약 ID (쉼표 구분)
- `cf_prohibit_email` - 금지 이메일 도메인 (줄바꿈 구분)
