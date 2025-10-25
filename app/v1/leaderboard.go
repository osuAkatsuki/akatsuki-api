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
		var (
			userDB            leaderboardUserDB
			chosenMode        modeData
			userID            int
			userName          string
			registerDateTime  common.UnixTimestamp
			privileges        uint64
			latestActivity    common.UnixTimestamp
			userNameAka       *string
			country           string
			userTitle         *string
			playStyle         int
			favouriteMode     int
		)
		err := rows.Scan(
			&userID, &userName, &registerDateTime, &privileges, &latestActivity,
			&userNameAka, &country, &userTitle, &playStyle, &favouriteMode,
			&chosenMode.RankedScore, &chosenMode.TotalScore, &chosenMode.PlayCount,
			&chosenMode.ReplaysWatched, &chosenMode.TotalHits,
			&chosenMode.Accuracy, &chosenMode.PP, &chosenMode.PPTotal, &chosenMode.PPStdDev,
		)
		if err != nil {
			md.Err(err)
			continue
		}
		chosenMode.Level = ocl.GetLevelPrecise(int64(chosenMode.TotalScore))

		userDB.userDataDB = userDataDB{
			ID:             userID,
			Username:       userName,
			RegisterDate:   registerDateTime,
			Privileges:     privileges,
			LatestActivity: latestActivity,
			UsernameAKA:    userNameAka,
			Country:        country,
			UserTitle:      userTitle,
		}
		userDB.PlayStyle = playStyle
		userDB.FavouriteMode = favouriteMode

		eligibleTitles := getEligibleTitles(md, userID)

		user := userDB.toLeaderboardUser(eligibleTitles)
		user.ChosenMode = chosenMode
		users = append(users, user)
	}

	return users
}

func leaderboardPosition(md common.MethodData, mode string, user int) int {
	return _position(md, "leaderboard:"+mode, user)
}

func countryPosition(md common.MethodData, mode string, user int, country string) int {
	return _position(md, "leaderboard:"+mode+":"+strings.ToLower(country), user)
}

func relaxboardPosition(md common.MethodData, mode string, user int) int {
	return _position(md, "relaxboard:"+mode, user)
}

func rxcountryPosition(md common.MethodData, mode string, user int, country string) int {
	return _position(md, "relaxboard:"+mode+":"+strings.ToLower(country), user)
}

func autoboardPosition(md common.MethodData, mode string, user int) int {
	return _position(md, "autoboard:"+mode, user)
}

func apcountryPosition(md common.MethodData, mode string, user int, country string) int {
	return _position(md, "autoboard:"+mode+":"+strings.ToLower(country), user)
}

func _position(md common.MethodData, key string, user int) int {
	val := md.R.ZRevRank(key, strconv.Itoa(user)).Val()
	return int(val) + 1
}

// LeaderboardGET retrieves leaderboard
func LeaderboardGET(md common.MethodData) common.CodeMessager {
	m := genModeClauseBoardGET(md)
	if m == nil {
		return *m
	}

	var resp leaderboardResponse
	resp.Code = 200

	p := md.Query("p")
	if p == "" {
		p = "0"
	}
	page, err := strconv.Atoi(p)
	if err != nil {
		return ErrBadJSON
	}

	l := md.Query("l")
	if l == "" {
		l = "50"
	}
	limit, err := strconv.Atoi(l)
	if err != nil {
		return ErrBadJSON
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}

	rx := 0
	if md.Query("rx") == "1" {
		rx = 1
	} else if md.Query("ap") == "1" {
		rx = 2
	}

	sort := md.Query("sort")
	if sort == "" {
		sort = "pp"
	}

	modeInt := 0
	mode := md.Query("mode")
	if mode != "" {
		modeInt, _ = strconv.Atoi(mode)
	}

	resp.Users = getLbUsersDb(page, limit, rx, modeInt, sort, md)

	return resp
}
