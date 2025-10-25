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

		var country string
		err := rows.Scan(
			&userDB.ID, &userDB.Username, &userDB.RegisteredOn, &userDB.Privileges, &userDB.LatestActivity,
			&userDB.UsernameAKA, &country, &userDB.UserTitle, &userDB.PlayStyle, &userDB.FavouriteMode,
			&chosenMode.RankedScore, &chosenMode.TotalScore, &chosenMode.PlayCount, &chosenMode.ReplaysWatched, &chosenMode.TotalHits,
			&chosenMode.Accuracy, &chosenMode.PP, &chosenMode.PPTotal, &chosenMode.PPStddev,
		)

		if err != nil {
			md.Err(err)
			continue
		}

		userDB.Country = strings.ToLower(country)

		eligibleTitles, err := getEligibleTitles(md, userDB.ID)
		if err != nil {
			md.Err(err)
			continue
		}

		user := userDB.toLeaderboardUser(eligibleTitles)
		user.ChosenMode = chosenMode
		users = append(users, user)
	}

	return users
}

// LeaderboardGET retrieves user leaderboard data
func LeaderboardGET(md common.MethodData) common.CodeMessager {
	m := genmodei(md.Query("mode"))

	rx := common.Int(md.Query("rx"))

	var sort string
	if md.Query("sort") == "score" {
		sort = "score"
	} else if md.Query("sort") == "pp_total" {
		sort = "pp_total"
	} else if md.Query("sort") == "pp_stddev" {
		sort = "pp_stddev"
	} else {
		sort = "pp"
	}

	p := common.Int(md.Query("p")) - 1
	if p < 0 {
		p = 0
	}
	l := common.InString(1, md.Query("l"), 500, 50)

	users := getLbUsersDb(p, l, rx, m, sort, md)

	r := leaderboardResponse{}
	r.Code = 200
	r.Users = users
	return r
}
