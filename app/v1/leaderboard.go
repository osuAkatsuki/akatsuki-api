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
			user_stats.avg_accuracy, user_stats.pp,

			agg.pp_total_all_modes, agg.pp_stddev_all_modes,
			agg.pp_total_classic, agg.pp_stddev_classic,
			agg.pp_total_relax, agg.pp_stddev_relax,
			agg.pp_total_std, agg.pp_total_taiko, agg.pp_total_catch,
			agg.pp_stddev_std, agg.pp_stddev_taiko, agg.pp_stddev_catch,
			agg.pp_std, agg.pp_std_rx, agg.pp_std_ap, agg.pp_taiko, agg.pp_taiko_rx, agg.pp_catch, agg.pp_catch_rx, agg.pp_mania
		FROM users
		INNER JOIN user_stats ON user_stats.user_id = users.id 
		LEFT JOIN player_pp_aggregates agg ON agg.player_id = users.id`

// previously done horrible hardcoding makes this the spaghetti it is
func getLbUsersDb(p int, l int, rx int, modeInt int, sort string, md common.MethodData) []leaderboardUser {
	var query, order string
	if sort == "score" {
		order = "ORDER BY user_stats.ranked_score DESC, user_stats.pp DESC"
	} else if sort == "pp_total_all_modes" {
		order = "ORDER BY agg.pp_total_all_modes DESC, user_stats.ranked_score DESC"
	} else if sort == "pp_stddev_all_modes" {
		order = "ORDER BY agg.pp_stddev_all_modes DESC, user_stats.ranked_score DESC"
	} else if sort == "pp_total_classic" {
		order = "ORDER BY agg.pp_total_classic DESC, user_stats.ranked_score DESC"
	} else if sort == "pp_stddev_classic" {
		order = "ORDER BY agg.pp_stddev_classic DESC, user_stats.ranked_score DESC"
	} else if sort == "pp_total_relax" {
		order = "ORDER BY agg.pp_total_relax DESC, user_stats.ranked_score DESC"
	} else if sort == "pp_stddev_relax" {
		order = "ORDER BY agg.pp_stddev_relax DESC, user_stats.ranked_score DESC"
	} else if sort == "pp_total_std" {
		order = "ORDER BY agg.pp_total_std DESC, user_stats.ranked_score DESC"
	} else if sort == "pp_mode" {
		// choose field by modeInt and rx
		field := "agg.pp_std"
		if modeInt == 0 {
			if rx == 1 {
				field = "agg.pp_std_rx"
			} else if rx == 2 {
				field = "agg.pp_std_ap"
			} else {
				field = "agg.pp_std"
			}
		} else if modeInt == 1 {
			if rx == 1 {
				field = "agg.pp_taiko_rx"
			} else {
				field = "agg.pp_taiko"
			}
		} else if modeInt == 2 {
			if rx == 1 {
				field = "agg.pp_catch_rx"
			} else {
				field = "agg.pp_catch"
			}
		} else if modeInt == 3 {
			field = "agg.pp_mania"
		}
		order = "ORDER BY " + field + " DESC, user_stats.ranked_score DESC"
	} else {
		order = "ORDER BY user_stats.pp DESC, user_stats.ranked_score DESC"
	}

	query = fmt.Sprintf(lbUserQuery+" WHERE (users.privileges & 3) >= 3 AND user_stats.mode = ? "+order+" LIMIT %d, %d", p*l, l)
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
		var ppTotalAll, ppStddevAll int
		var ppTotalClassic, ppStddevClassic int
		var ppTotalRelax, ppStddevRelax int
		var ppTotalStd, ppTotalTaiko, ppTotalCatch int
		var ppStddevStd, ppStddevTaiko, ppStddevCatch int
		var ppStd, ppStdRx, ppStdAp, ppTaiko, ppTaikoRx, ppCatch, ppCatchRx, ppMania int

		err := rows.Scan(
			&userDB.ID, &userDB.Username, &userDB.RegisteredOn, &userDB.Privileges, &userDB.LatestActivity,

			&userDB.UsernameAKA, &userDB.Country, &userDB.UserTitle, &userDB.PlayStyle, &userDB.FavouriteMode,

			&chosenMode.RankedScore, &chosenMode.TotalScore, &chosenMode.PlayCount,
			&chosenMode.ReplaysWatched, &chosenMode.TotalHits,
			&chosenMode.Accuracy, &chosenMode.PP,

			&ppTotalAll, &ppStddevAll,
			&ppTotalClassic, &ppStddevClassic,
			&ppTotalRelax, &ppStddevRelax,
			&ppTotalStd, &ppTotalTaiko, &ppTotalCatch,
			&ppStddevStd, &ppStddevTaiko, &ppStddevCatch,
			&ppStd, &ppStdRx, &ppStdAp, &ppTaiko, &ppTaikoRx, &ppCatch, &ppCatchRx, &ppMania,
		)
		if err != nil {
			md.Err(err)
			continue
		}

		// Apply chosen PP based on mode/rx
		if modeInt == 0 {
			if rx == 1 {
				chosenMode.PPTotal = ppStdRx
			} else if rx == 2 {
				chosenMode.PPTotal = ppStdAp
			} else {
				chosenMode.PPTotal = ppStd
			}
		} else if modeInt == 1 {
			if rx == 1 {
				chosenMode.PPTotal = ppTaikoRx
			} else {
				chosenMode.PPTotal = ppTaiko
			}
		} else if modeInt == 2 {
			if rx == 1 {
				chosenMode.PPTotal = ppCatchRx
			} else {
				chosenMode.PPTotal = ppCatch
			}
		} else if modeInt == 3 {
			chosenMode.PPTotal = ppMania
		}

		chosenMode.Level = ocl.GetLevelPrecise(int64(chosenMode.TotalScore))

		var eligibleTitles []eligibleTitle
		eligibleTitles, err = getEligibleTitles(md, userDB.ID, userDB.Privileges)
		if err != nil {
			md.Err(err)
			return make([]leaderboardUser, 0)
		}

		// Convert to API response format
		u := userDB.toLeaderboardUser(eligibleTitles)
		u.ChosenMode = chosenMode

		users = append(users, u)
	}
	return users
}

// LeaderboardGET gets the leaderboard.
func LeaderboardGET(md common.MethodData) common.CodeMessager {
	m := getMode(md.Query("mode"))
	modeInt := getModeInt(m)

	// md.Query.Country
	p := common.Int(md.Query("p")) - 1
	if p < 0 {
		p = 0
	}
	l := common.InString(1, md.Query("l"), 500, 50)
	rx := common.Int(md.Query("rx"))
	sort := md.Query("sort")
	if sort == "" {
		sort = "pp"
	}

	if sort != "pp" && sort != "score" {
		resp := leaderboardResponse{Users: getLbUsersDb(p, l, rx, modeInt, sort, md)}
		resp.Code = 200
		return resp
	}
	key := "ripple:leaderboard:" + m
	if rx == 1 {
		key = "ripple:relaxboard:" + m
	} else if rx == 2 {
		key = "ripple:autoboard:" + m
	}
	if md.Query("country") != "" {
		key += ":" + strings.ToLower(md.Query("country"))
	}

	results, err := md.R.ZRevRange(key, int64(p*l), int64(p*l+l-1)).Result()
	if err != nil {
		md.Err(err)
		return Err500
	}

	var resp leaderboardResponse
	resp.Code = 200
	if len(results) == 0 {
		return resp
	}

	var query = lbUserQuery + ` WHERE users.id IN (?) AND user_stats.mode = ? ORDER BY user_stats.pp DESC, user_stats.ranked_score DESC`
	query, params, _ := sqlx.In(query, results, modeInt+(rx*4))
	rows, err := md.DB.Query(query, params...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()
	for rows.Next() {
		userDB := leaderboardUserDB{}
		var chosenMode modeData
		var ppTotalAll, ppStddevAll int
		var ppTotalClassic, ppStddevClassic int
		var ppTotalRelax, ppStddevRelax int
		var ppTotalStd, ppTotalTaiko, ppTotalCatch int
		var ppStddevStd, ppStddevTaiko, ppStddevCatch int
		var ppStd, ppStdRx, ppStdAp, ppTaiko, ppTaikoRx, ppCatch, ppCatchRx, ppMania int

		err := rows.Scan(
			&userDB.ID, &userDB.Username, &userDB.RegisteredOn, &userDB.Privileges, &userDB.LatestActivity,

			&userDB.UsernameAKA, &userDB.Country, &userDB.UserTitle, &userDB.PlayStyle, &userDB.FavouriteMode,

			&chosenMode.RankedScore, &chosenMode.TotalScore, &chosenMode.PlayCount,
			&chosenMode.ReplaysWatched, &chosenMode.TotalHits,
			&chosenMode.Accuracy, &chosenMode.PP,

			&ppTotalAll, &ppStddevAll,
			&ppTotalClassic, &ppStddevClassic,
			&ppTotalRelax, &ppStddevRelax,
			&ppTotalStd, &ppTotalTaiko, &ppTotalCatch,
			&ppStddevStd, &ppStddevTaiko, &ppStddevCatch,
			&ppStd, &ppStdRx, &ppStdAp, &ppTaiko, &ppTaikoRx, &ppCatch, &ppCatchRx, &ppMania,
		)
		if err != nil {
			md.Err(err)
			continue
		}

		// Apply chosen PP based on mode/rx
		if modeInt == 0 {
			if rx == 1 {
				chosenMode.PPTotal = ppStdRx
			} else if rx == 2 {
				chosenMode.PPTotal = ppStdAp
			} else {
				chosenMode.PPTotal = ppStd
			}
		} else if modeInt == 1 {
			if rx == 1 {
				chosenMode.PPTotal = ppTaikoRx
			} else {
				chosenMode.PPTotal = ppTaiko
			}
		} else if modeInt == 2 {
			if rx == 1 {
				chosenMode.PPTotal = ppCatchRx
			} else {
				chosenMode.PPTotal = ppCatch
			}
		} else if modeInt == 3 {
			chosenMode.PPTotal = ppMania
		}

		chosenMode.Level = ocl.GetLevelPrecise(int64(chosenMode.TotalScore))

		var eligibleTitles []eligibleTitle
		eligibleTitles, err = getEligibleTitles(md, userDB.ID, userDB.Privileges)
		if err != nil {
			md.Err(err)
			return Err500
		}

		// Convert to API response format
		u := userDB.toLeaderboardUser(eligibleTitles)
		u.ChosenMode = chosenMode
		if rx == 1 {
			if i := relaxboardPosition(md.R, m, u.ID); i != nil {
				u.ChosenMode.GlobalLeaderboardRank = i
			}
			if i := rxcountryPosition(md.R, m, u.ID, u.Country); i != nil {
				u.ChosenMode.CountryLeaderboardRank = i
			}
		} else if rx == 2 {
			if i := autoboardPosition(md.R, m, u.ID); i != nil {
				u.ChosenMode.GlobalLeaderboardRank = i
			}
			if i := apcountryPosition(md.R, m, u.ID, u.Country); i != nil {
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
