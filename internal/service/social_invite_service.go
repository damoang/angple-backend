package service

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
	pkglogger "github.com/damoang/angple-backend/pkg/logger"
)

type SocialInviteService struct {
	repo *repository.SocialInviteRepository
}

func NewSocialInviteService(repo *repository.SocialInviteRepository) *SocialInviteService {
	return &SocialInviteService{repo: repo}
}

func generateSocialInviteToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("토큰 생성 실패: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *SocialInviteService) CreateInvite(targetMbID string, adminID string) (*domain.SocialInviteCreateResponse, error) {
	targetMbID = strings.TrimSpace(targetMbID)
	if targetMbID == "" {
		return nil, fmt.Errorf("대상 회원 ID가 필요합니다")
	}

	member, err := s.repo.FindMemberByID(targetMbID)
	if err != nil {
		return nil, fmt.Errorf("회원 '%s'을(를) 찾을 수 없습니다", targetMbID)
	}
	if member.MbLeaveDate != "" {
		return nil, fmt.Errorf("대상 계정이 탈퇴 상태입니다. 먼저 탈퇴 해제 후 초대 링크를 생성해주세요.")
	}

	token, err := generateSocialInviteToken()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(12 * time.Hour)
	invite := &domain.SocialInvite{
		Token:        token,
		TargetMbID:   targetMbID,
		TargetMbNick: member.MbNick,
		CreatedBy:    adminID,
		ExpiresAt:    expiresAt,
	}
	if err := s.repo.Create(invite); err != nil {
		return nil, fmt.Errorf("초대 생성 실패: %w", err)
	}

	s.writeLog(targetMbID, adminID, "social_invite_create", fmt.Sprintf("토큰: %s...", token[:8]))

	url := fmt.Sprintf("https://damoang.net/invite/%s", token)
	emailCert := fmt.Sprintf("안녕하세요, 다모앙 개인정보책임자입니다.\n\n회원님의 아이디가 실명 인증 처리 및 복원이 완료되었습니다.\n아래 링크에서 소셜 로그인을 연결해주시기 바랍니다.\n\n아이디 : %s\n\n소셜 로그인 연결 : %s\n\n감사합니다.", targetMbID, url)
	emailNoCert := fmt.Sprintf("안녕하세요, 다모앙 개인정보책임자입니다.\n\n회원님의 아이디가 복원이 완료되었습니다.\n아래 링크에서 소셜 로그인을 연결해주시기 바랍니다.\n\n아이디 : %s\n\n소셜 로그인 연결 : %s\n\n해당 아이디는 실명인증 하지 않는 것으로 나오는데, 실명인증 후에 사용부탁드립니다.\n\n감사합니다.", targetMbID, url)

	pkglogger.Info("[SocialInvite] invite created: target=%s, admin=%s, token=%s...", targetMbID, adminID, token[:8])

	return &domain.SocialInviteCreateResponse{
		Token:               token,
		URL:                 url,
		ExpiresAt:           expiresAt.Format(time.RFC3339),
		EmailTemplate:       emailCert,
		EmailTemplateNoCert: emailNoCert,
	}, nil
}

func (s *SocialInviteService) GetInviteInfo(token string, currentUserMbID string) (*domain.SocialInviteInfoResponse, error) {
	invite, err := s.repo.FindByToken(token)
	if err != nil {
		return nil, err
	}
	if invite == nil {
		return nil, fmt.Errorf("초대를 찾을 수 없습니다")
	}
	if invite.UsedAt != nil {
		return nil, fmt.Errorf("이미 사용된 초대입니다")
	}
	if time.Now().After(invite.ExpiresAt) {
		return nil, fmt.Errorf("만료된 초대입니다")
	}

	resp := &domain.SocialInviteInfoResponse{
		TargetMbID:   invite.TargetMbID,
		TargetMbNick: invite.TargetMbNick,
		ExpiresAt:    invite.ExpiresAt.Format(time.RFC3339),
	}

	if currentUserMbID != "" {
		resp.CurrentUserMbID = currentUserMbID
		if member, err := s.repo.FindMemberByID(currentUserMbID); err == nil {
			resp.CurrentUserMbNick = member.MbNick
		}
		if profiles, err := s.repo.FindSocialProfiles(currentUserMbID); err == nil {
			for _, p := range profiles {
				resp.CurrentSocials = append(resp.CurrentSocials, domain.SocialInviteRef{
					Provider:   p.Provider,
					SocialName: p.DisplayName,
				})
			}
		}
	}

	return resp, nil
}

func (s *SocialInviteService) ConfirmInvite(token string, currentUserMbID string) error {
	if currentUserMbID == "" {
		return fmt.Errorf("로그인이 필요합니다")
	}

	invite, err := s.repo.FindByToken(token)
	if err != nil {
		return err
	}
	if invite == nil {
		return fmt.Errorf("초대를 찾을 수 없습니다")
	}
	if invite.UsedAt != nil {
		return fmt.Errorf("이미 사용된 초대입니다")
	}
	if time.Now().After(invite.ExpiresAt) {
		return fmt.Errorf("만료된 초대입니다")
	}

	profiles, err := s.repo.FindSocialProfiles(currentUserMbID)
	if err != nil {
		return fmt.Errorf("소셜 프로필 조회 실패: %w", err)
	}
	if len(profiles) == 0 {
		return fmt.Errorf("이전할 소셜 프로필이 없습니다")
	}

	if err := s.repo.MarkUsed(token, currentUserMbID); err != nil {
		return fmt.Errorf("이미 사용된 초대입니다")
	}

	count := 0
	var transferErrors []string
	for _, p := range profiles {
		if err := s.repo.UpdateSocialProfileOwner(p.MpNo, invite.TargetMbID); err != nil {
			transferErrors = append(transferErrors, fmt.Sprintf("mp_no=%d: %v", p.MpNo, err))
			pkglogger.Error("[SocialInvite] failed to transfer profile mp_no=%d: %v", p.MpNo, err)
			continue
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("소셜 프로필 이전에 실패했습니다 (초대 토큰은 사용 처리됨, 관리자에게 문의하세요)")
	}
	if len(transferErrors) > 0 {
		pkglogger.Error("[SocialInvite] partial transfer: %d/%d succeeded, errors: %v", count, len(profiles), transferErrors)
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	_ = s.repo.AppendMemo(currentUserMbID, fmt.Sprintf("[%s] 소셜 프로필 %d개가 %s 계정으로 이전됨 (초대 링크)", now, count, invite.TargetMbID))
	_ = s.repo.AppendMemo(invite.TargetMbID, fmt.Sprintf("[%s] %s 계정에서 소셜 프로필 %d개 이전받음 (초대 링크)", now, currentUserMbID, count))

	details := fmt.Sprintf("%s → %s: 소셜 프로필 %d개 이전", currentUserMbID, invite.TargetMbID, count)
	s.writeLog(invite.TargetMbID, invite.CreatedBy, "social_invite_confirm", details)

	pkglogger.Info("[SocialInvite] confirmed: from=%s, to=%s, profiles=%d/%d", currentUserMbID, invite.TargetMbID, count, len(profiles))
	return nil
}

func (s *SocialInviteService) writeLog(mbID, adminID, action, details string) {
	_ = s.repo.WriteRecoveryLog(mbID, adminID, action, details)
}
