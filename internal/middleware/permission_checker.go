package middleware

import (
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// DBBoardPermissionChecker checks board permissions using database
type DBBoardPermissionChecker struct {
	boardRepo v2repo.BoardRepository
}

// NewDBBoardPermissionChecker creates a new permission checker
func NewDBBoardPermissionChecker(boardRepo v2repo.BoardRepository) *DBBoardPermissionChecker {
	return &DBBoardPermissionChecker{boardRepo: boardRepo}
}

func (c *DBBoardPermissionChecker) CanList(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.boardRepo.FindBySlug(boardSlug)
	if err != nil {
		return false, err
	}
	return memberLevel >= int(board.ListLevel), nil
}

func (c *DBBoardPermissionChecker) CanRead(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.boardRepo.FindBySlug(boardSlug)
	if err != nil {
		return false, err
	}
	return memberLevel >= int(board.ReadLevel), nil
}

func (c *DBBoardPermissionChecker) CanWrite(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.boardRepo.FindBySlug(boardSlug)
	if err != nil {
		return false, err
	}
	return memberLevel >= int(board.WriteLevel), nil
}

func (c *DBBoardPermissionChecker) CanReply(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.boardRepo.FindBySlug(boardSlug)
	if err != nil {
		return false, err
	}
	return memberLevel >= int(board.ReplyLevel), nil
}

func (c *DBBoardPermissionChecker) CanComment(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.boardRepo.FindBySlug(boardSlug)
	if err != nil {
		return false, err
	}
	return memberLevel >= int(board.CommentLevel), nil
}

func (c *DBBoardPermissionChecker) CanUpload(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.boardRepo.FindBySlug(boardSlug)
	if err != nil {
		return false, err
	}
	return memberLevel >= int(board.UploadLevel), nil
}

func (c *DBBoardPermissionChecker) CanDownload(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.boardRepo.FindBySlug(boardSlug)
	if err != nil {
		return false, err
	}
	return memberLevel >= int(board.DownloadLevel), nil
}

func (c *DBBoardPermissionChecker) GetRequiredLevel(boardSlug string, action string) int {
	board, err := c.boardRepo.FindBySlug(boardSlug)
	if err != nil {
		return 1
	}
	switch action {
	case "list":
		return int(board.ListLevel)
	case "read":
		return int(board.ReadLevel)
	case "write":
		return int(board.WriteLevel)
	case "reply":
		return int(board.ReplyLevel)
	case "comment":
		return int(board.CommentLevel)
	case "upload":
		return int(board.UploadLevel)
	case "download":
		return int(board.DownloadLevel)
	default:
		return 1
	}
}

func (c *DBBoardPermissionChecker) GetAllPermissions(boardSlug string, memberLevel int) (*BoardPermissions, error) {
	board, err := c.boardRepo.FindBySlug(boardSlug)
	if err != nil {
		return nil, err
	}
	return &BoardPermissions{
		CanList:     memberLevel >= int(board.ListLevel),
		CanRead:     memberLevel >= int(board.ReadLevel),
		CanWrite:    memberLevel >= int(board.WriteLevel),
		CanReply:    memberLevel >= int(board.ReplyLevel),
		CanComment:  memberLevel >= int(board.CommentLevel),
		CanUpload:   memberLevel >= int(board.UploadLevel),
		CanDownload: memberLevel >= int(board.DownloadLevel),
	}, nil
}
