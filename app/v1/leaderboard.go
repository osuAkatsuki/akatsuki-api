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
			chosenMode.GlobalLeaderboardRank = getRankCountry(modeInt+(rx*4), userDB.ID, country, sort, md)
		}

		user := userDB.toLeaderboardUser(getEligibleTitles(md))
		user.ChosenMode = chosenMode
		users = append(users, user)
	}
	return users
}

func getRankCountry(mode int, user int, country string, sort string, md common.MethodData) *int {
	var rank int
	var sortField string
	if sort == "pp_total" {
		sortField = "pp_total"
	} else if sort == "pp_stddev" {
		sortField = "pp_stddev"
	} else {
		sortField = "pp"
	}

	query := fmt.Sprintf(`SELECT COUNT(*) AS rank FROM user_stats INNER JOIN users ON users.id = user_stats.user_id WHERE user_stats.mode = ? AND user_stats.%s > (SELECT %s FROM user_stats WHERE user_id = ? AND mode = ?) AND users.country = ? AND (users.privileges & 3) >= 3`, sortField, sortField)
	if err := md.DB.QueryRow(query, mode, user, mode, country).Scan(&rank); err != nil {
		md.Err(err)
		return nil
	}
	rank++
	return &rank
}

func leaderboardPosition(r *redis.Client, mode string, user int) *int {
	return _position(r, "ripple:leaderboard:"+mode, user)
}

func countryPosition(r *redis.Client, mode string, user int, country string) *int {
	return _position(r, "ripple:leaderboard:"+mode+":"+strings.ToLower(country), user)
}

func relaxboardPosition(r *redis.Client, mode string, user int) *int {
	return _position(r, "ripple:relaxboard:"+mode, user)
}

func rxcountryPosition(r *redis.Client, mode string, user int, country string) *int {
	return _position(r, "ripple:relaxboard:"+mode+":"+strings.ToLower(country), user)
}

func autoboardPosition(r *redis.Client, mode string, user int) *int {
	return _position(r, "ripple:autoboard:"+mode, user)
}

func apcountryPosition(r *redis.Client, mode string, user int, country string) *int {
	return _position(r, "ripple:autoboard:"+mode+":"+strings.ToLower(country), user)
}

func _position(r *redis.Client, key string, user int) *int {
	res := r.ZRevRank(key, strconv.Itoa(user))
	if res.Err() == redis.Nil {
		return nil
	}
	x := int(res.Val()) + 1
	return &x
}
