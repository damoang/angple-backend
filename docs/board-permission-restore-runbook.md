# 게시판 권한 복원 런북 (프로덕션)

> v2_boards의 접근권한(list_level 등)이 초기 마이그레이션에서 누락되어
> 전부 0(공개)으로 들어간 것을 g5_board 원본값으로 복원한다.
> 관련 PR: angple-backend#550 (코드), 본 런북 (데이터).

## 전제
- PR #550 가 **먼저 프로덕션에 배포**되어 있어야 함
  (핸들러가 list_level 기준으로 필터하므로).
- migrate 바이너리는 이 커밋으로 빌드된 것 사용
  (`migrateBoards` 가 INSERT + UPDATE 재동기화 포함).

## 1) 실행 전: 무엇이 바뀌는지 미리보기 (읽기 전용 SELECT)

프로덕션 DB에서 실행 — **변경되는 보드 목록**:

```sql
SELECT g.bo_table              AS slug,
       g.bo_subject            AS name,
       v.list_level            AS v2_list_level_before,
       g.bo_list_level         AS g5_list_level_after,
       v.read_level            AS v2_read_before,
       g.bo_read_level         AS g5_read_after
FROM v2_boards v
JOIN g5_board g ON g.bo_table = v.slug
WHERE v.list_level  <> g.bo_list_level
   OR v.read_level  <> g.bo_read_level
ORDER BY g.bo_list_level DESC, g.bo_table;
```

특히 `g5_list_level_after > 0` 인 게시판들이 **목록에서 숨겨질 대상**이다.
(예상: archive/adm/disciplinelog/governance 등 관리·비공개 보드)

몇 개가 바뀌는지 카운트:
```sql
SELECT COUNT(*) FROM v2_boards v JOIN g5_board g ON g.bo_table=v.slug
WHERE v.list_level<>g.bo_list_level OR v.read_level<>g.bo_read_level;
```

## 2) 실행 (권한 컬럼만 복원)

```bash
# migrate 바이너리 있는 환경(백엔드 pod / 배포 호스트)에서
# DB_HOST/DB_USER/DB_PASSWORD/DB_NAME 환경변수 설정된 상태로:

./migrate -target boards -config configs/config.dev.yaml -verbose
# (config 는 프로덕션 config 경로로. env 가 실제 DB를 가리키면 됨)
```

- INSERT IGNORE (신규 보드) + UPDATE (기존 보드 권한 재동기화) 실행.
- 로그: `[migrate:boards] Inserted N new rows` / `Re-synced permissions on M rows`.
- **posts/comments/users 는 건드리지 않음** (target boards 한정).
- 안전성: 재실행해도 동일 결과(idempotent). 게시글 데이터 무관.

## 3) 실행 후 검증

```sql
-- 관리/비공개 보드가 이제 list_level>0 인지
SELECT slug, name, list_level, read_level FROM v2_boards
WHERE slug IN ('archive','adm','disciplinelog','governance','angtt','free','economy')
ORDER BY list_level DESC;
```

API 확인 (게스트 = 완전 공개만 보여야 함):
```bash
curl -s "https://api.damoang.net/api/v2/boards" \
  | python3 -c "import sys,json; d=json.load(sys.stdin)['data']; \
print('게스트 보드 수:', len(d)); \
[print(b['slug'], b['list_level']) for b in d if b['list_level']>0]"
# → list_level>0 인 게 출력되면 안 됨(전부 0이어야 게스트 노출 정상)
```

Redis 게스트 캐시 키가 `v2:boards:visible:guest:v2` 로 바뀌었으므로
배포 직후 자동 재생성됨(구 키 `v2:boards:active:v1` 는 무시됨).

## 롤백
- 데이터 롤백 불필요(권한 값을 원본으로 맞추는 작업). 문제 시 g5_board
  원본이 그대로이므로 다시 -target boards 실행하면 됨.
- 코드 롤백이 필요하면 #550 revert (그러면 다시 전 보드 노출로 회귀).
