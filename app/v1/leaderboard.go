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

type leaderboardUser struct {
	userData
	ChosenMode    modeData `json:"chosen_mode"`
	PlayStyle     int      `json:"play_style"`
	FavouriteMode int      `json:"favourite_mode"`
}

type leaderboardResponse struct {
	common.ResponseBase
	Users []leaderboardUser `json:"users"`
}

const lbUserQuery = `
		SELECT
			users.id, users.name, users.creation_time, users.priv, users.latest_activity,

			username_aka, country,
			play_style, favourite_mode,

			stats.rscore, stats.tscore, stats.plays,
			stats.replays_watched, total_hits,
			stats.acc, stats.pp
		FROM users
		INNER JOIN stats USING(id) WHERE stats.mode = %[1]s`

// previously done horrible hardcoding makes this the spaghetti it is
func getLbUsersDb(p, l, rx, m int, sort string, md *common.MethodData) []leaderboardUser {
	var query, order string
	if sort == "score" {
		order = "ORDER BY stats.rscore DESC, stats.pp DESC"
	} else {
		order = "ORDER BY stats.pp DESC, stats.rscore DESC"
	}

	relax_mode := rx + m

	query = fmt.Sprintf(lbUserQuery+"WHERE (users.privileges & 3) >= 3 "+order+" LIMIT %d, %d", relax_mode, p*l, l)
	rows, err := md.DB.Query(query)
	if err != nil {
		md.Err(err)
		return make([]leaderboardUser, 0)
	}
	defer rows.Close()
	var users []leaderboardUser
	for i := 1; rows.Next(); i++ {
		u := leaderboardUser{}
		err := rows.Scan(
			&u.ID, &u.Username, &u.RegisteredOn, &u.Privileges, &u.LatestActivity,

			&u.UsernameAKA, &u.Country, &u.PlayStyle, &u.FavouriteMode,

			&u.ChosenMode.RankedScore, &u.ChosenMode.TotalScore, &u.ChosenMode.PlayCount,
			&u.ChosenMode.ReplaysWatched, &u.ChosenMode.TotalHits,
			&u.ChosenMode.Accuracy, &u.ChosenMode.PP,
		)
		if err != nil {
			md.Err(err)
			continue
		}
		u.ChosenMode.Level = ocl.GetLevelPrecise(int64(u.ChosenMode.TotalScore))
		users = append(users, u)
	}
	return users
}

// LeaderboardGET gets the leaderboard.
func LeaderboardGET(md common.MethodData) common.CodeMessager {
	m := getMode(md.Query("mode"))

	// md.Query.Country
	p := common.Int(md.Query("p")) - 1
	if p < 0 {
		p = 0
	}
	l := common.InString(1, md.Query("l"), 500, 50)
	rx := md.Query("rx") == "1"
	sort := md.Query("sort")
	if sort == "" {
		sort = "pp"
	}

	if sort != "pp" {
		resp := leaderboardResponse{Users: getLbUsersDb(p, l, common.Int(md.Query("rx")), common.Int(md.Query("mode")), sort, &md)}
		resp.Code = 200
		return resp
	}
	key := "ripple:leaderboard:" + m
	if common.Int(md.Query("rx")) != 0 {
		key = "ripple:relaxboard:" + m
	}
	if md.Query("country") != "" {
		key += ":" + md.Query("country")
	}

	results, err := md.R.ZRevRange(key, int64(p*l), int64(p*l+l-1)).Result()
	if err != nil {
		md.Err(err)
		return Err500
	}

	var resp leaderboardResponse
	resp.Code = 200
	if len(results) == 0 {
		resp.Users = getLbUsersDb(p, l, common.Int(md.Query("rx")), common.Int(md.Query("mode")), sort, &md)
		return resp
	}

	var query string
	relax_mode := common.Int(md.Query("rx")) + common.Int(md.Query("mode"))

	query = fmt.Sprintf(lbUserQuery+`WHERE users.id IN (?) AND stats.mode = %[1]s ORDER BY stats.pp DESC, stats.rscore DESC`, relax_mode)
	query, params, _ := sqlx.In(query, results)
	rows, err := md.DB.Query(query, params...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()
	for rows.Next() {
		var u leaderboardUser
		err := rows.Scan(
			&u.ID, &u.Username, &u.RegisteredOn, &u.Privileges, &u.LatestActivity,

			&u.UsernameAKA, &u.Country, &u.PlayStyle, &u.FavouriteMode,

			&u.ChosenMode.RankedScore, &u.ChosenMode.TotalScore, &u.ChosenMode.PlayCount,
			&u.ChosenMode.ReplaysWatched, &u.ChosenMode.TotalHits,
			&u.ChosenMode.Accuracy, &u.ChosenMode.PP,
		)
		if err != nil {
			md.Err(err)
			continue
		}
		u.ChosenMode.Level = ocl.GetLevelPrecise(int64(u.ChosenMode.TotalScore))
		if rx {
			if i := relaxboardPosition(md.R, m, u.ID); i != nil {
				u.ChosenMode.GlobalLeaderboardRank = i
			}
			if i := rxcountryPosition(md.R, m, u.ID, u.Country); i != nil {
				u.ChosenMode.CountryLeaderboardRank = i
			}
		} else {
			if i := leaderboardPosition(md.R, m, u.ID); i != nil {
				u.ChosenMode.GlobalLeaderboardRank = i
			}
			if i := countryPosition(md.R, m, u.ID, u.Country); i != nil {
				u.ChosenMode.CountryLeaderboardRank = i
			}
		}
		resp.Users = append(resp.Users, u)
	}
	return resp
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
