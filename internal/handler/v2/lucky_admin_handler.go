package v2

import (
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/middleware"
	v2svc "github.com/damoang/angple-backend/internal/service/v2"
	"github.com/gin-gonic/gin"
)

// luckyAdminMaxBody 는 관리자 설정 본문 최대 크기다. 설정 JSON 은 수 KB 라 넉넉히 잡되 무한히 읽지 않는다.
const luckyAdminMaxBody = 256 << 10

// LuckyAdminHandler 는 앙팡(럭키 포인트) 관리자 화면 API 다. 라우트는 관리자 미들웨어 아래에만 둔다.
// ⛔ 어떤 응답에도 무작위 window 의 그날 시각을 싣지 않는다(서비스가 시각 키를 받지 않는다).
type LuckyAdminHandler struct {
	svc *v2svc.LuckyAdminService
	// onBoardsChanged 는 게시판 lucky 변경 뒤 게시판별 settings 전체로 부른다(nil-safe).
	// 기존 게시판 확장설정 저장과 같은 후처리(게시판 캐시 무효화·레거시 파일 동기화)를 하게 하려는 것이다.
	onBoardsChanged func(changed map[string]string)
}

// NewLuckyAdminHandler 는 LuckyAdminHandler 를 만든다.
func NewLuckyAdminHandler(svc *v2svc.LuckyAdminService) *LuckyAdminHandler {
	return &LuckyAdminHandler{svc: svc}
}

// SetOnBoardsChanged 는 게시판 일괄 변경 후처리를 주입한다.
func (h *LuckyAdminHandler) SetOnBoardsChanged(fn func(changed map[string]string)) {
	h.onBoardsChanged = fn
}

// GetConfig GET /api/v2/admin/lucky/config — 정규화된 lucky_config, 게시판별 lucky, 최근 변경 이력.
func (h *LuckyAdminHandler) GetConfig(c *gin.Context) {
	view, err := h.svc.GetConfig()
	if err != nil {
		log.Printf("[lucky-admin] config 조회 실패: %v", err)
		common.V2ErrorResponse(c, http.StatusInternalServerError, "설정을 읽지 못했습니다", err)
		return
	}
	common.V2Success(c, view)
}

// PutConfig PUT /api/v2/admin/lucky/config — lucky_config 전체 교체(검증·이력·이 파드 캐시 무효화).
func (h *LuckyAdminHandler) PutConfig(c *gin.Context) {
	body, ok := readLuckyAdminBody(c)
	if !ok {
		return
	}
	cfg, err := h.svc.PutConfig(body, luckyAdminActor(c))
	if err != nil {
		writeLuckyAdminError(c, "설정 저장", err)
		return
	}
	common.V2Success(c, gin.H{"config": cfg})
}

// PutBoards PUT /api/v2/admin/lucky/boards — 고른 게시판들의 lucky 키만 일괄 교체.
func (h *LuckyAdminHandler) PutBoards(c *gin.Context) {
	body, ok := readLuckyAdminBody(c)
	if !ok {
		return
	}
	req, changed, err := h.svc.PutBoards(body)
	if err != nil {
		writeLuckyAdminError(c, "게시판 저장", err)
		return
	}
	if h.onBoardsChanged != nil {
		h.onBoardsChanged(changed)
	}
	common.V2Success(c, gin.H{"board_ids": req.BoardIDs, "lucky": req.Lucky})
}

// GetStats GET /api/v2/admin/lucky/stats?date=YYYY-MM-DD — KST 하루 집계(회원 ID 대신 닉네임).
func (h *LuckyAdminHandler) GetStats(c *gin.Context) {
	st, err := h.svc.Stats(c.Query("date"))
	if err != nil {
		writeLuckyAdminError(c, "통계 조회", err)
		return
	}
	common.V2Success(c, st)
}

// luckyAdminActor 는 이력의 by 에 남길 mb_id 다. Bearer 경로에서 userID 는 v2_users.id 라 username 을 먼저 쓴다.
func luckyAdminActor(c *gin.Context) string {
	if u := middleware.GetUsername(c); u != "" {
		return u
	}
	return middleware.GetUserID(c)
}

// readLuckyAdminBody 는 본문을 크기 제한을 두고 읽는다. 실패하면 응답을 쓰고 ok=false.
func readLuckyAdminBody(c *gin.Context) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, luckyAdminMaxBody+1))
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 본문을 읽지 못했습니다", err)
		return nil, false
	}
	if len(body) > luckyAdminMaxBody {
		common.V2ErrorResponse(c, http.StatusRequestEntityTooLarge, "요청 본문이 너무 큽니다", nil)
		return nil, false
	}
	return body, true
}

// writeLuckyAdminError 는 검증 실패면 400 + 필드별 메시지, 그 밖에는 500 을 쓴다.
func writeLuckyAdminError(c *gin.Context, what string, err error) {
	var ve *v2svc.LuckyValidationError
	if errors.As(err, &ve) {
		c.JSON(http.StatusBadRequest, common.V2Response{
			Success: false,
			Error: &common.V2Error{
				Code:    "BAD_REQUEST",
				Message: "입력 값이 올바르지 않습니다",
				Details: ve.Fields,
			},
		})
		return
	}
	log.Printf("[lucky-admin] %s 실패: %v", what, err)
	common.V2ErrorResponse(c, http.StatusInternalServerError, what+"에 실패했습니다", err)
}
