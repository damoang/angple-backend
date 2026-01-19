package domain

import "time"

// Autosave represents an autosaved post draft (g5_autosave table)
type Autosave struct {
	ID        int       `gorm:"column:as_id;primaryKey;autoIncrement" json:"id"`
	MemberID  string    `gorm:"column:mb_id;index" json:"member_id"`
	UID       int       `gorm:"column:as_uid" json:"uid"`
	Subject   string    `gorm:"column:as_subject;size:255" json:"subject"`
	Content   string    `gorm:"column:as_content;type:text" json:"content"`
	CreatedAt time.Time `gorm:"column:as_datetime" json:"created_at"`
}

// TableName returns the table name for Autosave
func (Autosave) TableName() string {
	return "g5_autosave"
}

// AutosaveRequest represents request for saving a draft
type AutosaveRequest struct {
	UID     int    `json:"uid"`
	Subject string `json:"subject" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// AutosaveListItem represents an item in the autosave list
type AutosaveListItem struct {
	ID        int    `json:"id"`
	UID       int    `json:"uid"`
	Subject   string `json:"subject"`
	CreatedAt string `json:"created_at"`
}

// AutosaveDetail represents full autosave data
type AutosaveDetail struct {
	ID        int    `json:"id"`
	Subject   string `json:"subject"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// AutosaveResponse represents the response after saving
type AutosaveResponse struct {
	Count int `json:"count"`
}
