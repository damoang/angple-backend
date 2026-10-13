package stacksrv

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"time"
)

// 뱀 게임 기록 — 앙쌓기 혼자하기와 같은 방식(시작 시 runId+seed, 5초 간격, 2시간 만료, 1회용).
// 끝나면 클라이언트가 {runId, inputs, score} 를 보내고, 서버가 같은 규칙(snake_engine.go)으로
// seed+입력을 다시 돌려 점수를 맞춰 본다. 맞지 않는 판은 flagged 로 남기고 랭킹에서 뺀다.

// snakeBodyMax 는 finish 요청 본문 상한이다.
const snakeBodyMax = 128 << 10

// SnakeTimeSlackMs 는 재연 최소 시간이 서버 실경과를 넘어도 봐주는 고정 여유(ms)다(요청 왕복 등).
const SnakeTimeSlackMs = 2000

// SnakeTimeSlackPercent 는 실경과에 더해 주는 비율 여유다(브라우저 타이머 오차).
const SnakeTimeSlackPercent = 5

// snakeStoredScoreMax 는 저장할 신고 점수 상한이다(조작된 큰 값이 INT 열을 넘지 않게).
const snakeStoredScoreMax = SnakeGrid * SnakeGrid

// keyAccepted 는 finish 응답의 통과 여부 키다.
const keyAccepted = "accepted"

// 검사 실패 사유 코드
const (
	snakeFlagScore    = "score_mismatch"
	snakeFlagTooFast  = "too_fast"
	snakeFlagBadScore = "score_shape"
)

// SnakeRecord 는 저장할 뱀 게임 판 하나다. Flagged 면 감사용으로만 남고 랭킹에서 빠진다.
type SnakeRecord struct {
	// RunID 는 발급한 판 id 다.
	RunID string
	// MbID 는 회원 아이디다.
	MbID string
	// Seed 는 발급한 시드다.
	Seed uint32
	// Score 는 점수다(통과한 판은 재연 점수와 같다).
	Score int64
	// Ticks 는 재연한 이동 수다.
	Ticks int64
	// InputsJSON 은 입력 기록 원본(JSON)이다.
	InputsJSON string
	// ElapsedMs 는 서버가 잰 실경과다.
	ElapsedMs int64
	// Flagged 는 검사 실패 여부다.
	Flagged bool
	// FlagReason 은 실패 사유 코드(쉼표 구분)다.
	FlagReason string
	// StartedAt 은 시작 시각이다.
	StartedAt time.Time
	// WeekStart 는 이 판이 속한 주(KST 월요일 0시)다.
	WeekStart time.Time
}

// SnakeStore 는 뱀 게임 기록 저장소다.
type SnakeStore interface {
	// SaveSnakeRun 은 판을 남기고, 검사를 통과했으면 최고·주간 기록을 갱신한 뒤 내 기록을 돌려준다.
	SaveSnakeRun(rec SnakeRecord) (SoloStanding, error)
}

// SnakeService 는 뱀 게임 기록 접수다. 판 발급은 혼자하기 접수기를 따로 하나 두고 그대로 쓴다
// (회원당 1개·5초 간격·2시간 만료·1회용 규칙이 같다, 앙쌓기 판과 섞이지 않는다).
type SnakeService struct {
	// issuer 는 판 발급기다(저장소 없이 발급·인증만 쓴다).
	issuer *SoloService
	// store 는 저장소다.
	store SnakeStore
}

// NewSnakeService 는 뱀 게임 접수를 만든다.
func NewSnakeService(store SnakeStore, verifyToken func(string) (string, string, error)) *SnakeService {
	return &SnakeService{issuer: NewSoloService(nil, verifyToken), store: store}
}

// HandleStart 는 POST /stack-ws/snake/start → {runId, seed} 다. 로그인 회원만.
func (s *SnakeService) HandleStart(w http.ResponseWriter, r *http.Request) {
	s.issuer.HandleStart(w, r)
}

// snakeFinishRequest 는 POST /snake/finish 본문이다. 숫자는 JSON 그대로(float64) 받아 정수인지 직접 본다.
type snakeFinishRequest struct {
	// RunID 는 start 에서 받은 판 id 다.
	RunID string `json:"runId"`
	// Inputs 는 입력 기록 [[틱, 방향], …] 이다.
	Inputs [][]float64 `json:"inputs"`
	// Score 는 신고 점수다.
	Score float64 `json:"score"`
}

// SnakeCheck 는 기록 검사 결과다.
type SnakeCheck struct {
	// Reasons 는 실패 사유 코드다(통과면 비어 있다).
	Reasons []string
	// Replay 는 재연 결과다.
	Replay SnakeReplay
}

// OK 는 모든 검사를 통과했는지다.
func (c SnakeCheck) OK() bool { return len(c.Reasons) == 0 }

// SnakeClaimCheck 는 seed+입력을 재연해 신고 점수·실경과와 맞춰 본다.
// 사유: 입력 오류(shape·input_order·input_dir·input_after_end) / score_shape / score_mismatch /
// too_fast(재연 최소 시간 > 실경과 × 1.05 + 2초).
func SnakeClaimCheck(seed uint32, inputs [][]float64, score float64, elapsedMs int64) SnakeCheck {
	rep := ReplaySnake(seed, inputs)
	var reasons []string
	if rep.Error != "" {
		reasons = append(reasons, rep.Error)
	}
	if !isIntIn(score, 0, snakeStoredScoreMax) {
		reasons = append(reasons, snakeFlagBadScore)
	} else if rep.Error == "" && int(score) != rep.Score {
		reasons = append(reasons, snakeFlagScore)
	}
	if elapsedMs < 0 {
		elapsedMs = 0
	}
	if rep.SimMs > elapsedMs+elapsedMs*SnakeTimeSlackPercent/100+SnakeTimeSlackMs {
		reasons = append(reasons, snakeFlagTooFast)
	}
	return SnakeCheck{Reasons: reasons, Replay: rep}
}

// storedSnakeScore 는 저장할 점수다. 정수가 아니거나 범위를 넘으면 0~상한으로 자른다.
func storedSnakeScore(score float64) int64 {
	if math.IsNaN(score) || score < 0 {
		return 0
	}
	if score > snakeStoredScoreMax {
		return snakeStoredScoreMax
	}
	return int64(score)
}

// HandleFinish 는 POST /stack-ws/snake/finish {runId, inputs, score} → {accepted, best, weekBest, rankWeek?} 다.
// 검사를 통과하지 못한 판은 flagged 로 남기고 랭킹에서 뺀다. 같은 runId 는 한 번만 받는다.
func (s *SnakeService) HandleFinish(w http.ResponseWriter, r *http.Request) {
	mbID, req, ok := s.readFinish(w, r)
	if !ok {
		return
	}
	run := s.issuer.take(mbID, req.RunID)
	if run == nil {
		writeJSON(w, http.StatusConflict, map[string]interface{}{keyAccepted: false, keyError: "unknown_run"})
		return
	}
	standing, accepted, err := s.finish(run, req)
	if err != nil {
		log.Printf("[stack] snake save failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{keyAccepted: false, keyError: "save_failed"})
		return
	}
	writeJSON(w, http.StatusOK, standingBody(standing, accepted))
}

// readFinish 는 메서드·인증·본문(128KB·입력 길이 상한)을 확인한다. 실패면 응답을 쓰고 false.
func (s *SnakeService) readFinish(w http.ResponseWriter, r *http.Request) (string, snakeFinishRequest, bool) {
	var req snakeFinishRequest
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return "", req, false
	}
	mbID, authed := s.issuer.authenticate(r)
	if !authed {
		writeError(w, http.StatusUnauthorized, codeUnauthorized)
		return "", req, false
	}
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, snakeBodyMax)).Decode(&req)
	if err != nil || len(req.Inputs) > SnakeMaxTicks {
		writeError(w, http.StatusBadRequest, "bad_request")
		return "", req, false
	}
	return mbID, req, true
}

// standingBody 는 finish 응답 본문이다.
func standingBody(st SoloStanding, accepted bool) map[string]interface{} {
	body := map[string]interface{}{keyAccepted: accepted, "best": st.Best, "weekBest": st.WeekBest}
	if st.RankWeek > 0 {
		body["rankWeek"] = st.RankWeek
	}
	return body
}

// finish 는 재연 검사하고 저장한다. 통과 여부와 저장 뒤 내 기록을 돌려준다.
// 실경과는 서버가 판을 발급한 시각부터 지금까지다(클라이언트 시계는 믿지 않는다).
func (s *SnakeService) finish(run *soloRun, req snakeFinishRequest) (SoloStanding, bool, error) {
	now := s.issuer.now()
	rec := SnakeRecord{
		RunID: run.id, MbID: run.mbID, Seed: run.seed, Score: storedSnakeScore(req.Score),
		ElapsedMs: now.Sub(run.startedAt).Milliseconds(), StartedAt: run.startedAt, WeekStart: WeekStartKST(now),
		InputsJSON: "[]",
	}
	check := SnakeClaimCheck(run.seed, req.Inputs, req.Score, rec.ElapsedMs)
	rec.Ticks = int64(check.Replay.Ticks)
	rec.Flagged = !check.OK()
	rec.FlagReason = joinReasons(check.Reasons)
	if raw, err := json.Marshal(req.Inputs); err == nil {
		rec.InputsJSON = string(raw)
	}
	if s.store == nil {
		return SoloStanding{}, !rec.Flagged, nil
	}
	standing, err := s.store.SaveSnakeRun(rec)
	return standing, !rec.Flagged, err
}
