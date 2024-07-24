package v1

import (
	"database/sql"

	"github.com/osuAkatsuki/akatsuki-api/common"
)

type userGrades struct {
	XHCount int `json:"xh_count"`
	XCount  int `json:"x_count"`
	SHCount int `json:"sh_count"`
	SCount  int `json:"s_count"`
	ACount  int `json:"a_count"`
	BCount  int `json:"b_count"`
	CCount  int `json:"c_count"`
	DCount  int `json:"d_count"`
}
type userGradesResponse struct {
	common.ResponseBase
	Grades userGrades `json:"grades"`
}

func UserGradesGET(md common.MethodData) common.CodeMessager {
	var response userGradesResponse
	mode := common.Int(md.Query("mode"))
	userID := common.Int(md.Query("id"))
	query := `
		SELECT
			xh_count, x_count, sh_count, s_count,
			a_count, b_count, c_count, d_count
		FROM user_stats
		WHERE user_id = ? AND user_stats.mode = ?
	`

	err := md.DB.QueryRow(query, userID, mode).Scan(
		&response.Grades.XHCount,
		&response.Grades.XCount,
		&response.Grades.SHCount,
		&response.Grades.SCount,
		&response.Grades.ACount,
		&response.Grades.BCount,
		&response.Grades.CCount,
		&response.Grades.DCount,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return common.SimpleResponse(404, "User stats not found")
		}
		md.Err(err)
		return Err500
	}
	response.Code = 200
	return response
}
