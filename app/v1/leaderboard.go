package v1

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	redis "gopkg.in/redis.v5"

	"github.com/osuAkatsuki/akatsuki-api/common"
	"zxq.co/ripple/ocl"
)

// leaderboardUserDB is used for scanning from database
type leaderboardUserDB struct {
	userDataDB
	PlayStyle     int `db:"play_style"`
	FavouriteMode int `db:"favourite_mode"`
}

// leaderboardUser is used for API responses
type leaderboardUser struct {
	userData
	ChosenMode    modeData `json:"chosen_mode"`
	PlayStyle     int      `json:"play_style"`
	FavouriteMode int      `json:"favourite_mode"`
}

// toLeaderboardUser converts leaderboardUserDB to leaderboardUser
func (lub *leaderboardUserDB) toLeaderboardUser(eligibleTitles []eligibleTitle) leaderboardUser {
	return leaderboardUser{
		userData:      lub.userDataDB.toUserData(eligibleTitles),
		PlayStyle:     lub.PlayStyle,
		FavouriteMode: lub.FavouriteMode,
	}
}

type leaderboardResponse struct {
	common.ResponseBase
	Users []leaderboardUser `json:"users"`
}

const lbUserQuery = `
		SELECT
			users.id, users.username, users.register_datetime, users.privileges, users.latest_activity,
			users.username_aka, users.country, users.user_title, users.play_style, users.favourite_mode,
			user_stats.ranked_score, user_stats.total_score, user_stats.playcount,
			user_stats.replays_watched, user_stats.total_hits,
			user_stats.avg_accuracy, user_stats.pp, user_stats.pp_total, user_stats.pp_stddev
		FROM users
		INNER JOIN user_stats ON user_stats.user_id = users.id `

// previously done horrible hardcoding makes this the spaghetti it is
func getLbUsersDb(p int, l int, rx int, modeInt int, sort string, md common.MethodData) []leaderboardUser {
	var query, order string

	if sort == "score" {
		order = "ORDER BY user_stats.ranked_score DESC, user_stats.pp DESC"
	} else if sort == "pp_total" {
		order = "ORDER BY user_stats.pp_total DESC, user_stats.ranked_score DESC"
	} else if sort == "pp_stddev" {
		order = "ORDER BY user_stats.pp_stddev DESC, user_stats.ranked_score DESC"
	} else {
		order = "ORDER BY user_stats.pp DESC, user_stats.ranked_score DESC"
	}

	query = fmt.Sprintf(lbUserQuery+"WHERE (users.privileges & 3) >= 3 AND user_stats.mode = ? "+order+" LIMIT %d, %d", p*l, l)

	rows, err := md.DB.Query(query, modeInt+(rx*4))
	if err != nil {
		md.Err(err)
		return make([]leaderboardUser, 0)
	}
	defer rows.Close()

	var users []leaderboardUser
	for i := 1; rows.Next(); i++ {
		userDB := leaderboardUserDB{}
		var chosenMode modeData

		err := rows.Scan(
			&userDB.ID, &userDB.Username, &userDB.RegisteredOn, &userDB.Privileges, &userDB.LatestActivity,
			&userDB.UsernameAKA, &userDB.Country, &userDB.UserTitle, &userDB.PlayStyle, &userDB.FavouriteMode,
			&chosenMode.RankedScore, &chosenMode.TotalScore, &chosenMode.PlayCount,
			&chosenMode.ReplaysWatched, &chosenMode.TotalHits,
			&chosenMode.Accuracy, &chosenMode.PP, &chosenMode.PPTotal, &chosenMode.PPStddev,
		)
		if err != nil {
			md.Err(err)
			continue
		}

		chosenMode.Level = ocl.GetLevelPrecise(int64(chosenMode.TotalScore))

		if sort != "pp" && sort != "pp_total" && sort != "pp_stddev" {
			chosenMode.GlobalLeaderboardRank = &i
		} else {
			chosenMode.GlobalLeaderboardRank = getRank(modeInt+(rx*4), userDB.ID, md)
		}

		user := userDB.toLeaderboardUser(getEligibleTitles(md))
		user.ChosenMode = chosenMode
		users = append(users, user)
	}
	return users
}

func getRank(mode int, user int, md common.MethodData) *int {
	var rank int
	if err := md.DB.QueryRow(`SELECT COUNT(*) AS rank FROM user_stats WHERE mode = ? AND pp > (SELECT pp FROM user_stats WHERE user_id = ? AND mode = ?) AND (SELECT privileges FROM users WHERE id = user_id) & 3 >= 3`, mode, user, mode).Scan(&rank); err != nil {
		md.Err(err)
		return nil
	}
	rank++
	return &rank
}

const userQueryBase = `
SELECT
	users.id, users.username, users.register_datetime, users.privileges, users.latest_activity,
	users.username_aka, users.country, users.user_title
FROM users
WHERE users.id = ?
	AND (users.privileges & 3) >= 3
LIMIT 1
`

type leaderboardRequest struct {
	Mode      int
	Page      int
	Length    int
	Country   string
	Relax     int
	Sort      string
}

// LeaderboardGET retrieves the global leaderboard
func LeaderboardGET(md common.MethodData) common.CodeMessager {
	r := leaderboardRequest{}

	md.Unmarshal(&r)

	if r.Mode < 0 || r.Mode > 3 {
		r.Mode = 0
	}
	if r.Page < 0 {
		r.Page = 0
	}
	if r.Length < 1 || r.Length > 100 {
		r.Length = 50
	}
	if r.Sort == "" {
		r.Sort = "pp"
	}

	var users []leaderboardUser

	if r.Country != "" {
		users = getLbUsersCountry(r.Page, r.Length, r.Relax, r.Mode, strings.ToUpper(r.Country), r.Sort, md)
	} else {
		users = getLbUsersDb(r.Page, r.Length, r.Relax, r.Mode, r.Sort, md)
	}

	return leaderboardResponse{
		ResponseBase: common.ResponseBase{Code: 200},
		Users:        users,
	}
}

func getLbUsersCountry(p int, l int, rx int, modeInt int, country string, sort string, md common.MethodData) []leaderboardUser {
	var query, order string

	if sort == "score" {
		order = "ORDER BY user_stats.ranked_score DESC, user_stats.pp DESC"
	} else if sort == "pp_total" {
		order = "ORDER BY user_stats.pp_total DESC, user_stats.ranked_score DESC"
	} else if sort == "pp_stddev" {
		order = "ORDER BY user_stats.pp_stddev DESC, user_stats.ranked_score DESC"
	} else {
		order = "ORDER BY user_stats.pp DESC, user_stats.ranked_score DESC"
	}

	query = fmt.Sprintf(lbUserQuery+"WHERE (users.privileges & 3) >= 3 AND user_stats.mode = ? AND users.country = ? "+order+" LIMIT %d, %d", p*l, l)

	rows, err := md.DB.Query(query, modeInt+(rx*4), country)
	if err != nil {
		md.Err(err)
		return make([]leaderboardUser, 0)
	}
	defer rows.Close()

	var users []leaderboardUser
	for i := 1; rows.Next(); i++ {
		userDB := leaderboardUserDB{}
		var chosenMode modeData

		err := rows.Scan(
			&userDB.ID, &userDB.Username, &userDB.RegisteredOn, &userDB.Privileges, &userDB.LatestActivity,
			&userDB.UsernameAKA, &userDB.Country, &userDB.UserTitle, &userDB.PlayStyle, &userDB.FavouriteMode,
			&chosenMode.RankedScore, &chosenMode.TotalScore, &chosenMode.PlayCount,
			&chosenMode.ReplaysWatched, &chosenMode.TotalHits,
			&chosenMode.Accuracy, &chosenMode.PP, &chosenMode.PPTotal, &chosenMode.PPStddev,
		)
		if err != nil {
			md.Err(err)
			continue
		}

		chosenMode.Level = ocl.GetLevelPrecise(int64(chosenMode.TotalScore))

		if sort != "pp" && sort != "pp_total" && sort != "pp_stddev" {
			chosenMode.GlobalLeaderboardRank = &i
		} else {
			chosenMode.GlobalLeaderboardRank = getRankCountry(modeInt+(rx*4), userDB.ID, country, md)
		}

		user := userDB.toLeaderboardUser(getEligibleTitles(md))
		user.ChosenMode = chosenMode
		users = append(users, user)
	}
	return users
}

func getRankCountry(mode int, user int, country string, md common.MethodData) *int {
	var rank int
	if err := md.DB.QueryRow(`SELECT COUNT(*) AS rank FROM user_stats INNER JOIN users ON users.id = user_stats.user_id WHERE user_stats.mode = ? AND user_stats.pp > (SELECT pp FROM user_stats WHERE user_id = ? AND mode = ?) AND users.country = ? AND (users.privileges & 3) >= 3`, mode, user, mode, country).Scan(&rank); err != nil {
		md.Err(err)
		return nil
	}
	rank++
	return &rank
}
