package domain

// DajoongiItem represents a duplicate account detection item
type DajoongiItem struct {
	IP        string `json:"ip"`
	MemberIDs string `json:"member_ids"`
	Boards    string `json:"boards"`
	Count     int    `json:"count"`
}

// DajoongiResponse represents the response for dajoongi list
type DajoongiResponse struct {
	Date  string          `json:"date"`
	Items []DajoongiItem  `json:"items"`
	Total int             `json:"total"`
}
