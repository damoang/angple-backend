package domain

// AdminMemberListItem represents a member in admin list view
type AdminMemberListItem struct {
	UserID        string `json:"user_id"`
	Nickname      string `json:"nickname"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	CreatedAt     string `json:"created_at"`
	LoginIP       string `json:"login_ip"`
	InterceptDate string `json:"intercept_date,omitempty"`
	ID            int    `json:"id"`
	Level         int    `json:"level"`
	Point         int    `json:"point"`
}

// AdminMemberDetail represents detailed member info for admin
type AdminMemberDetail struct {
	UserID        string `json:"user_id"`
	Nickname      string `json:"nickname"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	Birth         string `json:"birth,omitempty"`
	Homepage      string `json:"homepage,omitempty"`
	Profile       string `json:"profile,omitempty"`
	Signature     string `json:"signature,omitempty"`
	Memo          string `json:"memo,omitempty"`
	CreatedAt     string `json:"created_at"`
	TodayLogin    string `json:"today_login"`
	LoginIP       string `json:"login_ip"`
	IP            string `json:"ip"`
	InterceptDate string `json:"intercept_date,omitempty"`
	LeaveDate     string `json:"leave_date,omitempty"`
	Addr1         string `json:"addr1,omitempty"`
	Addr2         string `json:"addr2,omitempty"`
	ID            int    `json:"id"`
	Level         int    `json:"level"`
	Point         int    `json:"point"`
	MemoCount     int    `json:"memo_count"`
	ScrapCount    int    `json:"scrap_count"`
}

// ToAdminListItem converts Member to AdminMemberListItem
func (m *Member) ToAdminListItem() *AdminMemberListItem {
	return &AdminMemberListItem{
		ID:            m.ID,
		UserID:        m.UserID,
		Nickname:      m.Nickname,
		Name:          m.Name,
		Email:         m.Email,
		Level:         m.Level,
		Point:         m.Point,
		CreatedAt:     m.CreatedAt.Format("2006-01-02 15:04:05"),
		LoginIP:       m.LoginIP,
		InterceptDate: m.InterceptDate,
	}
}

// ToAdminDetail converts Member to AdminMemberDetail
func (m *Member) ToAdminDetail() *AdminMemberDetail {
	return &AdminMemberDetail{
		ID:            m.ID,
		UserID:        m.UserID,
		Nickname:      m.Nickname,
		Name:          m.Name,
		Email:         m.Email,
		Phone:         m.Phone,
		Birth:         m.Birth,
		Homepage:      m.Homepage,
		Profile:       m.Profile,
		Signature:     m.Signature,
		Memo:          m.Memo,
		Level:         m.Level,
		Point:         m.Point,
		CreatedAt:     m.CreatedAt.Format("2006-01-02 15:04:05"),
		TodayLogin:    m.TodayLogin.Format("2006-01-02 15:04:05"),
		LoginIP:       m.LoginIP,
		IP:            m.IP,
		InterceptDate: m.InterceptDate,
		LeaveDate:     m.LeaveDate,
		Addr1:         m.Addr1,
		Addr2:         m.Addr2,
		MemoCount:     m.MemoCount,
		ScrapCount:    m.ScrapCount,
	}
}

// AdminMemberUpdateRequest request for admin member update
type AdminMemberUpdateRequest struct {
	Nickname *string `json:"nickname,omitempty"`
	Name     *string `json:"name,omitempty"`
	Email    *string `json:"email,omitempty"`
	Level    *int    `json:"level,omitempty"`
	Memo     *string `json:"memo,omitempty"`
}

// AdminPointAdjustRequest request for adjusting member point
type AdminPointAdjustRequest struct {
	Point   int    `json:"point" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// AdminRestrictRequest request for restricting/unrestricting a member
type AdminRestrictRequest struct {
	InterceptDate string `json:"intercept_date"` // "YYYY-MM-DD" or "" to lift
	Reason        string `json:"reason"`
}
