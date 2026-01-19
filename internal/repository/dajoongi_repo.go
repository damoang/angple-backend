package repository

import (
	"regexp"
	"strings"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// DajoongiRepository handles dajoongi data operations
type DajoongiRepository struct {
	db *gorm.DB
}

// NewDajoongiRepository creates a new DajoongiRepository
func NewDajoongiRepository(db *gorm.DB) *DajoongiRepository {
	return &DajoongiRepository{db: db}
}

// DajoongiRawResult is the raw query result
type DajoongiRawResult struct {
	WrIP      string `gorm:"column:wr_ip"`
	DupMbIDs  string `gorm:"column:dup_mb_ids"`
	DupBdNm   string `gorm:"column:dup_bd_nm"`
	Cnt       int    `gorm:"column:cnt"`
}

// GetDuplicateAccounts retrieves list of IPs with multiple member IDs in the last N days
func (r *DajoongiRepository) GetDuplicateAccounts(days int) ([]domain.DajoongiItem, error) {
	var results []DajoongiRawResult

	query := `
		SELECT
			wr_ip,
			GROUP_CONCAT(DISTINCT mb_id) AS dup_mb_ids,
			GROUP_CONCAT(DISTINCT bo_table) AS dup_bd_nm,
			COUNT(1) AS cnt
		FROM
			g5_board_new
		WHERE
			mb_id NOT IN ('', 'admin')
			AND bn_datetime >= CONCAT(DATE_SUB(CURDATE(), INTERVAL ? DAY), ' 00:00:00')
		GROUP BY
			wr_ip
		HAVING
			COUNT(DISTINCT mb_id) > 1
		ORDER BY cnt DESC
	`

	if err := r.db.Raw(query, days).Scan(&results).Error; err != nil {
		return nil, err
	}

	items := make([]domain.DajoongiItem, 0, len(results))
	for _, result := range results {
		items = append(items, domain.DajoongiItem{
			IP:        maskIP(result.WrIP),
			MemberIDs: result.DupMbIDs,
			Boards:    result.DupBdNm,
			Count:     result.Cnt,
		})
	}

	return items, nil
}

// maskIP masks the IP address for privacy
func maskIP(ip string) string {
	// IPv4: xxx.xxx.xxx.xxx -> xxx.xxx.***.***
	ipv4Regex := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)\.(\d+)$`)
	if ipv4Regex.MatchString(ip) {
		parts := strings.Split(ip, ".")
		if len(parts) == 4 {
			return parts[0] + "." + parts[1] + ".***.***"
		}
	}

	// IPv6: mask last 4 segments
	if strings.Contains(ip, ":") {
		parts := strings.Split(ip, ":")
		if len(parts) >= 4 {
			for i := len(parts) - 4; i < len(parts); i++ {
				parts[i] = "****"
			}
			return strings.Join(parts, ":")
		}
	}

	return ip
}
