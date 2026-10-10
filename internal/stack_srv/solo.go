package stacksrv

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"sync"
	"time"
)

// SoloRunTTL 은 발급한 혼자하기 판(runId)의 수명이다.
const SoloRunTTL = 2 * time.Hour

// SoloStartInterval 은 회원당 판 시작 최소 간격이다.
const SoloStartInterval = 5 * time.Second

// SoloTickSlack 은 클라이언트 틱이 서버 실경과보다 앞설 수 있는 여유(틱)다.
const SoloTickSlack = 180

// DropRowsPerPieceMax 는 조각 하나가 내려올 수 있는 최대 줄 수다(판 높이 + 위로 올리는 회전 보정 12회).
const DropRowsPerPieceMax = Rows + 12

// soloClearsMax 는 clears 배열 길이 상한이다(요청 크기 방어).
const soloClearsMax = 20000

// soloBodyMax 는 finish 요청 본문 상한이다.
const soloBodyMax = 128 << 10

// lineScores 는 한 번에 지운 줄 수(0~4)별 기본 점수다. 레벨을 곱한다(웹 엔진과 같은 표).
var lineScores = [5]int64{0, 100, 250, 450, 700}

// errSoloRateLimited 는 판 시작이 너무 잦을 때다.
var errSoloRateLimited = errors.New("rate limited")

// ScoreFor 는 한 번에 지운 줄 수에 대한 점수다(웹 엔진 scoreFor 와 같다).
func ScoreFor(cleared int, level int64) int64 {
	if cleared < 0 || cleared > 4 {
		return 0
	}
	return lineScores[cleared] * level
}

// SoloClaim 은 혼자하기 기록 신고 값이다. 숫자는 JSON 그대로(float64) 받아 정수인지 직접 본다.
type SoloClaim struct {
	// Score 는 최종 점수다.
	Score float64 `json:"score"`
	// Lines 는 지운 줄 합이다.
	Lines float64 `json:"lines"`
	// Level 은 최종 레벨이다.
	Level float64 `json:"level"`
	// Ticks 는 진행한 고정 틱 수다.
	Ticks float64 `json:"ticks"`
	// Pieces 는 굳힌 조각 수다.
	Pieces float64 `json:"pieces"`
	// Clears 는 줄을 지운 순간마다 그때 지운 줄 수(1~4)다, 순서대로.
	Clears []float64 `json:"clears"`
}

// SoloCheck 는 SoloClaimCheck 의 결과다.
type SoloCheck struct {
	// Reasons 는 실패 사유 코드다(통과면 비어 있다).
	Reasons []string
	// ClearScore 는 재계산한 줄 점수다.
	ClearScore int64
	// DropScore 는 나머지(낙하 점수)다.
	DropScore int64
}

// OK 는 모든 검사를 통과했는지다.
func (r SoloCheck) OK() bool { return len(r.Reasons) == 0 }

// isCount 는 0 이상의 정수인지다.
func isCount(v float64) bool {
	return v >= 0 && v <= 1e12 && v == math.Trunc(v)
}

// toCount 는 저장용 정수다. 0 이상의 정수가 아니면(shape 실패) 0 으로 남긴다.
func toCount(v float64) int64 {
	if !isCount(v) {
		return 0
	}
	return int64(v)
}

// SoloClaimCheck 는 혼자하기 기록이 엔진 규칙상 가능한지 본다. 웹 versus.ts soloClaimCheck 와 같은 식이다.
// elapsedMs 는 서버가 잰 시작~끝 실경과다.
// 검사: clears 각 1~4·합=lines / level=1+floor(lines/8) / 0≤4×pieces−9×lines≤162 /
// clears 개수≤pieces≤ticks / ticks≤floor(elapsedMs×60/1000)+180 /
// 줄 점수 재계산(지울 때의 레벨)≤score, 나머지(낙하 점수)≤2×(18+12)×(pieces+1).
func SoloClaimCheck(c SoloClaim, elapsedMs int64) SoloCheck {
	if !claimShapeOK(c) {
		return SoloCheck{Reasons: []string{"shape"}}
	}
	score, lines, level := int64(c.Score), int64(c.Lines), int64(c.Level)
	ticks, pieces := int64(c.Ticks), int64(c.Pieces)

	sum, clearScore, rangeBad := scoreClears(c.Clears)
	var reasons []string
	if rangeBad {
		reasons = append(reasons, "clears_range")
	}
	if sum != lines {
		reasons = append(reasons, "lines_sum")
	}
	if level != 1+lines/LinesPerLevel {
		reasons = append(reasons, "level")
	}
	reasons = append(reasons, countReasons(lines, pieces, ticks, int64(len(c.Clears)), elapsedMs)...)
	drop := score - clearScore
	if r := dropReason(drop, pieces); r != "" {
		reasons = append(reasons, r)
	}
	return SoloCheck{Reasons: reasons, ClearScore: clearScore, DropScore: drop}
}

// claimShapeOK 는 숫자가 모두 0 이상의 정수이고 clears 가 있는지다.
func claimShapeOK(c SoloClaim) bool {
	return isCount(c.Score) && isCount(c.Lines) && isCount(c.Level) && isCount(c.Ticks) && isCount(c.Pieces) && c.Clears != nil
}

// scoreClears 는 clears 를 순서대로 훑어 줄 합과 줄 점수(지울 때의 레벨로)를 다시 계산한다.
// 1~4 정수가 아닌 값은 건너뛰고 rangeBad 로 알린다.
func scoreClears(clears []float64) (sum, clearScore int64, rangeBad bool) {
	for _, n := range clears {
		if n != math.Trunc(n) || n < 1 || n > 4 {
			rangeBad = true
			continue
		}
		levelAt := 1 + sum/LinesPerLevel
		clearScore += ScoreFor(int(n), levelAt)
		sum += int64(n)
	}
	return sum, clearScore, rangeBad
}

// countReasons 는 칸 보존·조각/틱·실경과 검사 실패 사유다(cells, pieces_ticks, ticks_elapsed 순서).
func countReasons(lines, pieces, ticks, clearEvents, elapsedMs int64) []string {
	var reasons []string
	if cells := 4*pieces - 9*lines; cells < 0 || cells > BoardCells {
		reasons = append(reasons, "cells")
	}
	if clearEvents > pieces || pieces > ticks {
		reasons = append(reasons, "pieces_ticks")
	}
	if elapsedMs < 0 {
		elapsedMs = 0
	}
	if ticks > elapsedMs*TicksPerSecond/1000+SoloTickSlack {
		reasons = append(reasons, "ticks_elapsed")
	}
	return reasons
}

// dropReason 은 낙하 점수(점수 − 줄 점수) 검사 실패 사유다. 통과면 빈 문자열.
func dropReason(drop, pieces int64) string {
	if drop < 0 {
		return "score_low"
	}
	if drop > 2*DropRowsPerPieceMax*(pieces+1) {
		return "drop_cap"
	}
	return ""
}

// SoloRecord 는 저장할 혼자하기 판 하나다. Flagged 면 감사용으로만 남고 점수판에서 빠진다.
type SoloRecord struct {
	// RunID 는 발급한 판 id 다.
	RunID string
	// MbID 는 회원 아이디다.
	MbID string
	// Seed 는 발급한 시드다.
	Seed uint32
	// Score 는 점수다.
	Score int64
	// Lines 는 지운 줄 합이다.
	Lines int64
	// Level 은 최종 레벨이다.
	Level int64
	// Ticks 는 틱 수다.
	Ticks int64
	// Pieces 는 굳힌 조각 수다.
	Pieces int64
	// ClearsJSON 은 clears 원본(JSON)이다.
	ClearsJSON string
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

// SoloStanding 은 저장 뒤의 내 기록이다.
type SoloStanding struct {
	// Best 는 역대 최고 점수다.
	Best int64
	// WeekBest 는 이번 주 최고 점수다.
	WeekBest int64
	// RankWeek 는 이번 주 순위다(기록 없으면 0).
	RankWeek int
}

// SoloStore 는 혼자하기 기록 저장소다.
type SoloStore interface {
	// SaveSoloRun 은 판을 남기고, 검사를 통과했으면 최고 기록·주간 기록을 갱신한 뒤 내 기록을 돌려준다.
	SaveSoloRun(rec SoloRecord) (SoloStanding, error)
}

// soloRun 은 발급한 판이다(메모리에만 있다).
type soloRun struct {
	// id 는 runId 다.
	id string
	// mbID 는 회원 아이디다.
	mbID string
	// seed 는 시드다.
	seed uint32
	// startedAt 은 발급 시각이다.
	startedAt time.Time
}

// SoloService 는 혼자하기 기록 접수다. runId 는 회원당 1개, 5초에 한 번만 발급한다.
type SoloService struct {
	// mu 는 아래 맵을 지킨다.
	mu sync.Mutex
	// runs 는 runId → 판이다.
	runs map[string]*soloRun
	// byMember 는 회원 아이디 → runId 다(회원당 1개).
	byMember map[string]string
	// lastStart 는 회원별 마지막 발급 시각이다.
	lastStart map[string]time.Time
	// store 는 저장소다.
	store SoloStore
	// verifyToken 은 JWT 검증기다.
	verifyToken func(string) (string, string, error)
	// now 는 시계다.
	now func() time.Time
}

// NewSoloService 는 혼자하기 접수를 만든다.
func NewSoloService(store SoloStore, verifyToken func(string) (string, string, error)) *SoloService {
	return &SoloService{runs: map[string]*soloRun{}, byMember: map[string]string{}, lastStart: map[string]time.Time{}, store: store, verifyToken: verifyToken, now: time.Now}
}

// Start 는 새 판을 발급한다. 같은 회원의 이전 판은 버린다.
func (s *SoloService) Start(mbID string) (string, uint32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if last, ok := s.lastStart[mbID]; ok && now.Sub(last) < SoloStartInterval {
		return "", 0, errSoloRateLimited
	}
	s.pruneLocked(now)
	if old, ok := s.byMember[mbID]; ok {
		delete(s.runs, old)
	}
	run := &soloRun{id: "r_" + generateID(20), mbID: mbID, seed: randomUint32(), startedAt: now}
	s.runs[run.id] = run
	s.byMember[mbID] = run.id
	s.lastStart[mbID] = now
	return run.id, run.seed, nil
}

// take 는 회원의 판을 꺼낸다(한 번만 쓸 수 있다). 없거나 남의 것이거나 만료면 nil.
func (s *SoloService) take(mbID, runID string) *soloRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[runID]
	if run == nil || run.mbID != mbID {
		return nil
	}
	delete(s.runs, runID)
	if s.byMember[mbID] == runID {
		delete(s.byMember, mbID)
	}
	if s.now().Sub(run.startedAt) > SoloRunTTL {
		return nil
	}
	return run
}

// pruneLocked 는 만료된 판과 오래된 발급 기록을 지운다.
func (s *SoloService) pruneLocked(now time.Time) {
	for id, run := range s.runs {
		if now.Sub(run.startedAt) > SoloRunTTL {
			delete(s.runs, id)
			if s.byMember[run.mbID] == id {
				delete(s.byMember, run.mbID)
			}
		}
	}
	for mb, t := range s.lastStart {
		if now.Sub(t) >= SoloStartInterval {
			delete(s.lastStart, mb)
		}
	}
}

// HandleStart 는 POST /stack-ws/solo/start → {runId, seed} 다. 로그인 회원만.
func (s *SoloService) HandleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	mbID, ok := s.authenticate(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	runID, seed, err := s.Start(mbID)
	if err != nil {
		writeError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"runId": runID, "seed": seed})
}

// soloFinishRequest 는 POST /solo/finish 본문이다.
type soloFinishRequest struct {
	SoloClaim
	// RunID 는 start 에서 받은 판 id 다.
	RunID string `json:"runId"`
}

// HandleFinish 는 POST /stack-ws/solo/finish {runId, score, lines, level, ticks, pieces, clears[]} →
// {accepted, best, weekBest, rankWeek?} 다. 검사를 통과하지 못한 판은 flagged 로 남기고 점수판에서 뺀다.
func (s *SoloService) HandleFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	mbID, ok := s.authenticate(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, codeUnauthorized)
		return
	}
	var req soloFinishRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, soloBodyMax)).Decode(&req); err != nil || len(req.Clears) > soloClearsMax {
		writeError(w, http.StatusBadRequest, "bad_request")
		return
	}
	run := s.take(mbID, req.RunID)
	if run == nil {
		writeJSON(w, http.StatusConflict, map[string]interface{}{"accepted": false, keyError: "unknown_run"})
		return
	}
	standing, accepted, err := s.finish(run, req.SoloClaim)
	if err != nil {
		log.Printf("[stack] solo save failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{"accepted": false, keyError: "save_failed"})
		return
	}
	body := map[string]interface{}{"accepted": accepted, "best": standing.Best, "weekBest": standing.WeekBest}
	if standing.RankWeek > 0 {
		body["rankWeek"] = standing.RankWeek
	}
	writeJSON(w, http.StatusOK, body)
}

// finish 는 검사하고 저장한다. 통과 여부와 저장 뒤 내 기록을 돌려준다.
func (s *SoloService) finish(run *soloRun, claim SoloClaim) (SoloStanding, bool, error) {
	now := s.now()
	elapsed := now.Sub(run.startedAt).Milliseconds()
	check := SoloClaimCheck(claim, elapsed)
	clearsJSON, err := json.Marshal(claim.Clears)
	if err != nil {
		clearsJSON = []byte("[]")
	}
	rec := SoloRecord{RunID: run.id, MbID: run.mbID, Seed: run.seed, Score: toCount(claim.Score), Lines: toCount(claim.Lines), Level: toCount(claim.Level), Ticks: toCount(claim.Ticks), Pieces: toCount(claim.Pieces), ClearsJSON: string(clearsJSON), ElapsedMs: elapsed, Flagged: !check.OK(), FlagReason: joinReasons(check.Reasons), StartedAt: run.startedAt, WeekStart: WeekStartKST(now)}
	if s.store == nil {
		return SoloStanding{}, check.OK(), nil
	}
	standing, err := s.store.SaveSoloRun(rec)
	return standing, check.OK(), err
}

// authenticate 는 Authorization: Bearer 토큰으로 회원을 확정한다.
func (s *SoloService) authenticate(r *http.Request) (string, bool) {
	token := bearerHeader(r)
	if token == "" || s.verifyToken == nil {
		return "", false
	}
	mbID, _, err := s.verifyToken(token)
	if err != nil || mbID == "" {
		return "", false
	}
	return mbID, true
}

// joinReasons 는 사유 코드를 쉼표로 잇는다.
func joinReasons(reasons []string) string {
	out := ""
	for i, r := range reasons {
		if i > 0 {
			out += ","
		}
		out += r
	}
	return out
}

// writeMethodNotAllowed 는 허용하지 않는 메서드에 405 를 쓴다.
func writeMethodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed")
}

// keyError 는 오류 응답의 키다.
const keyError = "error"

// writeError 는 {"error": code} 를 쓴다.
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]interface{}{keyError: code})
}

// writeJSON 은 개인 응답(캐시 금지) JSON 을 쓴다.
func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("[stack] write response failed: %v", err)
	}
}
