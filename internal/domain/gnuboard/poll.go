package gnuboard

import "time"

// G5Poll represents the g5_poll table
type G5Poll struct {
	PoID      int    `gorm:"column:po_id;primaryKey;autoIncrement" json:"po_id"`
	PoSubject string `gorm:"column:po_subject" json:"po_subject"`
	PoPoll1   string `gorm:"column:po_poll1" json:"po_poll1"`
	PoPoll2   string `gorm:"column:po_poll2" json:"po_poll2"`
	PoPoll3   string `gorm:"column:po_poll3" json:"po_poll3"`
	PoPoll4   string `gorm:"column:po_poll4" json:"po_poll4"`
	PoPoll5   string `gorm:"column:po_poll5" json:"po_poll5"`
	PoPoll6   string `gorm:"column:po_poll6" json:"po_poll6"`
	PoPoll7   string `gorm:"column:po_poll7" json:"po_poll7"`
	PoPoll8   string `gorm:"column:po_poll8" json:"po_poll8"`
	PoPoll9   string `gorm:"column:po_poll9" json:"po_poll9"`
	PoCnt1    int    `gorm:"column:po_cnt1" json:"po_cnt1"`
	PoCnt2    int    `gorm:"column:po_cnt2" json:"po_cnt2"`
	PoCnt3    int    `gorm:"column:po_cnt3" json:"po_cnt3"`
	PoCnt4    int    `gorm:"column:po_cnt4" json:"po_cnt4"`
	PoCnt5    int    `gorm:"column:po_cnt5" json:"po_cnt5"`
	PoCnt6    int    `gorm:"column:po_cnt6" json:"po_cnt6"`
	PoCnt7    int    `gorm:"column:po_cnt7" json:"po_cnt7"`
	PoCnt8    int    `gorm:"column:po_cnt8" json:"po_cnt8"`
	PoCnt9    int    `gorm:"column:po_cnt9" json:"po_cnt9"`
	PoEtc     string `gorm:"column:po_etc" json:"po_etc"`
	PoLevel   int    `gorm:"column:po_level" json:"po_level"`
	PoPoint   int    `gorm:"column:po_point" json:"po_point"`
	PoDate    string `gorm:"column:po_date" json:"po_date"`
	PoIPs     string `gorm:"column:po_ips" json:"-"`
	MbIDs     string `gorm:"column:mb_ids" json:"-"`
	PoUse     int    `gorm:"column:po_use" json:"po_use"`
}

// TableName returns the table name for GORM
func (G5Poll) TableName() string {
	return "g5_poll"
}

// G5PollEtc represents the g5_poll_etc table (user-submitted custom options)
type G5PollEtc struct {
	PcID       int       `gorm:"column:pc_id;primaryKey;autoIncrement" json:"pc_id"`
	PoID       int       `gorm:"column:po_id" json:"po_id"`
	MbID       string    `gorm:"column:mb_id" json:"mb_id"`
	PcName     string    `gorm:"column:pc_name" json:"pc_name"`
	PcIdea     string    `gorm:"column:pc_idea" json:"pc_idea"`
	PcDatetime time.Time `gorm:"column:pc_datetime" json:"pc_datetime"`
}

// TableName returns the table name for GORM
func (G5PollEtc) TableName() string {
	return "g5_poll_etc"
}

// PollOption represents a single poll option in the API response
type PollOption struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
	Count int    `json:"count"`
}

// PollResponse is the API response format for a poll
type PollResponse struct {
	ID        int          `json:"id"`
	Subject   string       `json:"subject"`
	Options   []PollOption `json:"options"`
	TotalVote int          `json:"total_vote"`
	Level     int          `json:"level"`
	Point     int          `json:"point"`
	Date      string       `json:"date"`
	IsActive  bool         `json:"is_active"`
	HasVoted  bool         `json:"has_voted"`
}

// ToPollResponse converts G5Poll to API response format
func (p *G5Poll) ToPollResponse(hasVoted bool) PollResponse {
	options := make([]PollOption, 0, 9)
	polls := []string{p.PoPoll1, p.PoPoll2, p.PoPoll3, p.PoPoll4, p.PoPoll5, p.PoPoll6, p.PoPoll7, p.PoPoll8, p.PoPoll9}
	counts := []int{p.PoCnt1, p.PoCnt2, p.PoCnt3, p.PoCnt4, p.PoCnt5, p.PoCnt6, p.PoCnt7, p.PoCnt8, p.PoCnt9}

	totalVote := 0
	for i := 0; i < 9; i++ {
		if polls[i] != "" {
			options = append(options, PollOption{
				Index: i + 1,
				Text:  polls[i],
				Count: counts[i],
			})
			totalVote += counts[i]
		}
	}

	return PollResponse{
		ID:        p.PoID,
		Subject:   p.PoSubject,
		Options:   options,
		TotalVote: totalVote,
		Level:     p.PoLevel,
		Point:     p.PoPoint,
		Date:      p.PoDate,
		IsActive:  p.PoUse == 1,
		HasVoted:  hasVoted,
	}
}
