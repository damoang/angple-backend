-- 회원 탈퇴 사유 컬럼 추가 (#feat/leave-reason-recording)
-- 값 목록: self(본인탈퇴), admin(관리자처리), terms_violation(약관위반),
--          contract_withdrawal(계약철회/개인정보보호법), account_abuse(계정도용/악용), other(기타)
ALTER TABLE g5_member
  ADD COLUMN mb_leave_reason VARCHAR(50) NOT NULL DEFAULT '' AFTER mb_leave_date;

-- diynbetterlife(naver_9c950964) 탈퇴 날짜 오기록 보정 (05.01 → 20260511)
UPDATE g5_member
SET mb_leave_date   = '20260511',
    mb_leave_reason = 'admin'
WHERE mb_id = 'naver_9c950964'
  AND mb_leave_date != '';
