package v1

import (
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"

	redis "gopkg.in/redis.v5"

	"github.com/osuAkatsuki/akatsuki-api/common"
	"zxq.co/ripple/ocl"
)

type SleaderboardUser struct {
	userData
	ChosenMode    modeData `json:"chosen_mode"`
	PlayStyle     int      `json:"play_style"`
	FavouriteMode int      `json:"favourite_mode"`
}

type SleaderboardResponse struct {
	common.ResponseBase
	Users []SleaderboardUser `json:"users"`
}

const SlbUserQuery = `
		SELECT
			users.id, users.username, users.register_datetime, users.privileges, users.latest_activity,
			users.username_aka, users.country, users.play_style, users.favourite_mode,

			user_stats.ranked_score, user_stats.total_score, user_stats.playcount,
			user_stats.replays_watched, user_stats.total_hits,
			user_stats.avg_accuracy
		FROM user_stats
		JOIN users ON users.id = user_stats.user_id
		WHERE user_stats.user_id IN (?) AND user_stats.mode = ?
		`

// LeaderboardGET gets the leaderboard.
func SLeaderboardGET(md common.MethodData) common.CodeMessager {
	m := getMode(md.Query("mode"))
	rx := common.Int(md.Query("rx"))

	// md.Query.Country
	p := common.Int(md.Query("p")) - 1
	if p < 0 {
		p = 0
	}
	l := common.InString(1, md.Query("l"), 500, 50)

	key := "ripple:leaderboard:" + m
	if rx == 1 {
		key = "ripple:relaxboard:" + m
	} else if rx == 2 {
		key = "ripple:autoboard:" + m
	}
	if md.Query("country") != "" {
		key += ":" + md.Query("country")
	}

	results, err := md.R.ZRevRange(key, int64(p*l), int64(p*l+l-1)).Result()
	if err != nil {
		md.Err(err)
		return Err500
	}

	var resp SleaderboardResponse
	resp.Code = 200

	if len(results) == 0 {
		return resp
	}

	query := lbUserQuery + ` ORDER BY user_stats.ranked_score DESC`
	query, params, _ := sqlx.In(query, results)
	rows, err := md.DB.Query(query, params...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	for rows.Next() {
		var u SleaderboardUser
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
		if rx == 1 {
			if i := SrelaxboardPosition(md.R, m, u.ID); i != nil {
				u.ChosenMode.GlobalLeaderboardRank = i
			}
			if i := SrxcountryPosition(md.R, m, u.ID, u.Country); i != nil {
				u.ChosenMode.CountryLeaderboardRank = i
			}
		} else if rx == 2 {
			if i := SautoboardPosition(md.R, m, u.ID); i != nil {
				u.ChosenMode.GlobalLeaderboardRank = i
			}
			if i := SapcountryPosition(md.R, m, u.ID, u.Country); i != nil {
				u.ChosenMode.CountryLeaderboardRank = i
			}
		} else {
			if i := SleaderboardPosition(md.R, m, u.ID); i != nil {
				u.ChosenMode.GlobalLeaderboardRank = i
			}
			if i := ScountryPosition(md.R, m, u.ID, u.Country); i != nil {
				u.ChosenMode.CountryLeaderboardRank = i
			}
		}
		resp.Users = append(resp.Users, u)
	}
	return resp
}

func SleaderboardPosition(r *redis.Client, mode string, user int) *int {
	return S_position(r, "ripple:leaderboard:"+mode, user)
}

func ScountryPosition(r *redis.Client, mode string, user int, country string) *int {
	return S_position(r, "ripple:leaderboard:"+mode+":"+strings.ToLower(country), user)
}

func SrelaxboardPosition(r *redis.Client, mode string, user int) *int {
	return _position(r, "ripple:relaxboard:"+mode, user)
}

func SrxcountryPosition(r *redis.Client, mode string, user int, country string) *int {
	return _position(r, "ripple:relaxboard:"+mode+":"+strings.ToLower(country), user)
}

func SautoboardPosition(r *redis.Client, mode string, user int) *int {
	return _position(r, "ripple:autoboard:"+mode, user)
}

func SapcountryPosition(r *redis.Client, mode string, user int, country string) *int {
	return _position(r, "ripple:autoboard:"+mode+":"+strings.ToLower(country), user)
}

func S_position(r *redis.Client, key string, user int) *int {
	res := r.ZRevRank(key, strconv.Itoa(user))
	if res.Err() == redis.Nil {
		return nil
	}
	x := int(res.Val()) + 1
	return &x
}
