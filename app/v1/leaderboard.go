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
	}
	// overlay: add sorting for individual pp fields based on mode/rx
	if sort == "pp_mode" {
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
	}
	// end overlay

	if rx == 2 {
		query = "SELECT * FROM (" + lbUserQuery + ") as t WHERE 1=1"
	} else if rx == 1 {
		query = "SELECT * FROM (" + lbUserQuery + ") as t WHERE 1=1"
	} else {
		query = "SELECT * FROM (" + lbUserQuery + ") as t WHERE 1=1"
	}

	query += " " + order

	rows, err := md.DB.Queryx(query)
	if err != nil {
		md.Err(err)
		return []leaderboardUser{}
	}
	defer rows.Close()

	resp := leaderboardResponse{}
	for rows.Next() {
		var userDB leaderboardUserDB
		var chosenMode modeData
		var ppTotalAll, ppStddevAll int
		var ppTotalClassic, ppStddevClassic int
		var ppTotalRelax, ppStddevRelax int
		var ppTotalStd, ppTotalTaiko, ppTotalCatch int
		var ppStddevStd, ppStddevTaiko, ppStddevCatch int
		// overlay: new fields
		var ppStd, ppStdRx, ppStdAp, ppTaiko, ppTaikoRx, ppCatch, ppCatchRx, ppMania int
		// end overlay

		err = rows.Scan(
			&userDB.ID, &userDB.Username, &userDB.RegisteredOn, &userDB.Privileges, &userDB.LatestActivity,
			&userDB.UsernameAKA, &userDB.Country, &userDB.UserTitle, &userDB.PlayStyle, &userDB.FavouriteMode,
			&chosenMode.RankedScore, &chosenMode.TotalScore, &chosenMode.Playcount,
			&chosenMode.ReplaysWatched, &chosenMode.TotalHits,
			&chosenMode.AvgAccuracy, &chosenMode.PP,
			&ppTotalAll, &ppStddevAll,
			&ppTotalClassic, &ppStddevClassic,
			&ppTotalRelax, &ppStddevRelax,
			&ppTotalStd, &ppTotalTaiko, &ppTotalCatch,
			&ppStddevStd, &ppStddevTaiko, &ppStddevCatch,
			// overlay: scan new fields
			&ppStd, &ppStdRx, &ppStdAp, &ppTaiko, &ppTaikoRx, &ppCatch, &ppCatchRx, &ppMania,
		)
		if err != nil {
			md.Err(err)
			continue
		}

		// overlay: apply chosen PP based on mode/rx
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
		// end overlay

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
