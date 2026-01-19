package v1

import (
	"database/sql"
	"strconv"
	"time"

	"golang.org/x/exp/slog"

	"github.com/jmoiron/sqlx"
	"github.com/osuAkatsuki/akatsuki-api/common"
)

// Achievement represents an achievement in the database.
type Achievement struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Icon        string  `json:"icon"`
	Mode        *int    `json:"mode"` // Game mode (0=std, 1=taiko, 2=catch, 3=mania, nil=all modes)
}

// LoadAchievementsEvery reloads the achievements in the database every given
// amount of time.
func LoadAchievementsEvery(db *sqlx.DB, d time.Duration) {
	for {
		achievs = nil
		err := db.Select(&achievs,
			"SELECT id, name, `desc` AS description, file AS icon, mode FROM less_achievements ORDER BY id ASC")
		if err != nil {
			slog.Error("LoadAchievements error", "error", err.Error())
			common.GenericError(err)
		}
		time.Sleep(d)
	}
}

var achievs []Achievement

type userAchievement struct {
	Achievement
	Achieved bool `json:"achieved"`
}

type userAchievementsResponse struct {
	common.ResponseBase
	Achievements []userAchievement `json:"achievements"`
}

// UserAchievementsGET handles requests for retrieving the achievements of a
// given user.
func UserAchievementsGET(md common.MethodData) common.CodeMessager {
	shouldRet, whereClause, param := whereClauseUser(md, "users")
	if shouldRet != nil {
		return *shouldRet
	}

	modeQuery := md.Query("mode")
	var ids []int
	var err error
	var fullMode int       // Full mode (0-8) for querying users_achievementsautopilot=8
	var vanillaMode *int   // Vanilla mode (0-3) for filtering less_achievements

	if modeQuery != "" {
		parsedMode, parseErr := strconv.Atoi(modeQuery)
		if parseErr == nil && parsedMode >= 0 && parsedMode <= 8 {
			fullMode = parsedMode
			vm := fullMode % 4
			vanillaMode = &vm
			err = md.DB.Select(&ids, `SELECT ua.achievement_id FROM users_achievements ua
INNER JOIN users ON users.id = ua.user_id
WHERE `+whereClause+` AND ua.mode = ? ORDER BY ua.achievement_id ASC`, param, fullMode)
		} else {
			// Invalid mode, return empty
			err = sql.ErrNoRows
		}
	} else {
		err = md.DB.Select(&ids, `SELECT ua.achievement_id FROM users_achievements ua
INNER JOIN users ON users.id = ua.user_id
WHERE `+whereClause+` ORDER BY ua.achievement_id ASC`, param)
	}

	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "No such user!")
	case err != nil:
		md.Err(err)
		return Err500
	}
	all := md.HasQuery("all")
	resp := userAchievementsResponse{Achievements: make([]userAchievement, 0, len(achievs))}

	for _, ach := range achievs {
		// Skip achievements that don't match the requested vanilla mode
		if vanillaMode != nil && ach.Mode != nil && *ach.Mode != *vanillaMode {
			continue
		}

		achieved := inInt(ach.ID, ids)
		if all || achieved {
			resp.Achievements = append(resp.Achievements, userAchievement{ach, achieved})
		}
	}
	resp.Code = 200
	return resp
}

func inInt(i int, js []int) bool {
	for _, j := range js {
		if i == j {
			return true
		}
	}
	return false
}
