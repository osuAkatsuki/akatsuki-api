// Package v1 implements the first version of the Ripple API.
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
			user_stats.avg_accuracy, user_stats.pp
		FROM users
		INNER JOIN user_stats ON user_stats.user_id = users.id `

const lbUserQueryWithAggregates = `
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
			&chosenMode.Accuracy, &chosenMode.PP,
		)
		if err != nil {
			md.Err(err)
			continue
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

	// For standard sorts (pp/score) use old logic
	if sort == "pp" || sort == "score" {
		return getStandardLeaderboard(m, modeInt, p, l, rx, sort, md)
	}

	// For new sorts (total/spp) use new logic with aggregates
	return getAggregateLeaderboard(m, modeInt, p, l, rx, sort, md)
}

func getStandardLeaderboard(m string, modeInt, p, l, rx int, sort string, md common.MethodData) common.CodeMessager {
	if sort != "pp" {
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

	var query = lbUserQuery + `WHERE users.id IN (?) AND user_stats.mode = ? ORDER BY user_stats.pp DESC, user_stats.ranked_score DESC`
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

		err := rows.Scan(
			&userDB.ID, &userDB.Username, &userDB.RegisteredOn, &userDB.Privileges, &userDB.LatestActivity,

			&userDB.UsernameAKA, &userDB.Country, &userDB.UserTitle, &userDB.PlayStyle, &userDB.FavouriteMode,

			&chosenMode.RankedScore, &chosenMode.TotalScore, &chosenMode.PlayCount,
			&chosenMode.ReplaysWatched, &chosenMode.TotalHits,
			&chosenMode.Accuracy, &chosenMode.PP,
		)
		if err != nil {
			md.Err(err)
			continue
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

func getAggregateLeaderboard(m string, modeInt, p, l, rx int, sort string, md common.MethodData) common.CodeMessager {
	var order string
	
	// Clean approach: simple sort=total and sort=spp
	switch sort {
	case "total":
		order = getTotalOrder(modeInt, rx)
	case "spp":
		order = getStdDevOrder(modeInt, rx)
	default:
		// For backward compatibility can add old formats if needed
		order = "ORDER BY user_stats.pp DESC, users.id ASC"
	}

	query := fmt.Sprintf(lbUserQueryWithAggregates+` 
		WHERE (users.privileges & 3) >= 3 
		AND user_stats.mode = ? 
		%s 
		LIMIT %d, %d`, order, p*l, l)

	rows, err := md.DB.Query(query, modeInt+(rx*4))
	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()

	var resp leaderboardResponse
	resp.Code = 200

	// Scan rows with aggregates
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

		// Apply PPTotal based on mode/rx
		applyPPTotal(&chosenMode, modeInt, rx, ppStd, ppStdRx, ppStdAp, ppTaiko, ppTaikoRx, ppCatch, ppCatchRx, ppMania)

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

		// For aggregate sorts use calculated positions
		globalRank := p*l + 1
		u.ChosenMode.GlobalLeaderboardRank = &globalRank
		// Country positions not supported for aggregate sorts

		resp.Users = append(resp.Users, u)
	}
	return resp
}

func getTotalOrder(modeInt, rx int) string {
	// Determine field for Total PP based on mode and rx
	if modeInt >= 0 && rx >= 0 {
		// Specific mode + specific rx
		return fmt.Sprintf("ORDER BY %s DESC, users.id ASC", getIndividualPPField(modeInt, rx))
	} else if modeInt >= 0 {
		// Specific mode, all rx - sum individual fields
		switch modeInt {
		case 0: // osu! all rx: std + std_rx + std_ap
			return "ORDER BY (agg.pp_std + agg.pp_std_rx + agg.pp_std_ap) DESC, users.id ASC"
		case 1: // taiko all rx: taiko + taiko_rx  
			return "ORDER BY (agg.pp_taiko + agg.pp_taiko_rx) DESC, users.id ASC"
		case 2: // catch all rx: catch + catch_rx
			return "ORDER BY (agg.pp_catch + agg.pp_catch_rx) DESC, users.id ASC"
		case 3: // mania
			return "ORDER BY agg.pp_mania DESC, users.id ASC"
		default: 
			return "ORDER BY agg.pp_total_all_modes DESC, users.id ASC"
		}
	} else if rx >= 0 {
		// All modes for specific rx
		switch rx {
		case 0: return "ORDER BY agg.pp_total_classic DESC, users.id ASC"      // vanilla
		case 1: return "ORDER BY agg.pp_total_relax DESC, users.id ASC"        // relax
		case 2: return "ORDER BY agg.pp_total_all_modes DESC, users.id ASC"    // autopilot
		default: return "ORDER BY agg.pp_total_all_modes DESC, users.id ASC"
		}
	} else {
		// Total overall (no filters)
		return "ORDER BY agg.pp_total_all_modes DESC, users.id ASC"
	}
}

func getStdDevOrder(modeInt, rx int) string {
	// Determine field for SPP based on mode and rx
	if modeInt >= 0 && rx >= 0 {
		// Specific mode + specific rx
		return fmt.Sprintf("ORDER BY %s DESC, users.id ASC", getIndividualStdDevField(modeInt, rx))
	} else if modeInt >= 0 {
		// Specific mode, all rx
		switch modeInt {
		case 0: return "ORDER BY agg.pp_stddev_std DESC, users.id ASC"     // osu! all rx
		case 1: return "ORDER BY agg.pp_stddev_taiko DESC, users.id ASC"   // taiko all rx
		case 2: return "ORDER BY agg.pp_stddev_catch DESC, users.id ASC"   // catch all rx
		default: return "ORDER BY agg.pp_stddev_classic DESC, users.id ASC" // mania
		}
	} else if rx >= 0 {
		// All modes for specific rx
		switch rx {
		case 0: return "ORDER BY agg.pp_stddev_classic DESC, users.id ASC"     // vanilla
		case 1: return "ORDER BY agg.pp_stddev_relax DESC, users.id ASC"       // relax
		case 2: return "ORDER BY agg.pp_stddev_all_modes DESC, users.id ASC"   // autopilot
		default: return "ORDER BY agg.pp_stddev_all_modes DESC, users.id ASC"
		}
	} else {
		// Total overall (no filters)
		return "ORDER BY agg.pp_stddev_all_modes DESC, users.id ASC"
	}
}

func getIndividualPPField(modeInt, rx int) string {
	// Individual PP fields for specific mode and rx
	switch modeInt {
	case 0: // osu!
		switch rx {
		case 0: return "agg.pp_std"     // vanilla
		case 1: return "agg.pp_std_rx"  // relax
		case 2: return "agg.pp_std_ap"  // autopilot
		}
	case 1: // taiko
		switch rx {
		case 0: return "agg.pp_taiko"    // vanilla
		case 1: return "agg.pp_taiko_rx" // relax
		}
	case 2: // catch
		switch rx {
		case 0: return "agg.pp_catch"    // vanilla
		case 1: return "agg.pp_catch_rx" // relax
		}
	case 3: // mania
		return "agg.pp_mania" // only vanilla
	}
	return "user_stats.pp" // fallback
}

func getIndividualStdDevField(modeInt, rx int) string {
	// Individual StdDev fields for specific mode and rx
	switch rx {
	case 0: // vanilla
		switch modeInt {
		case 0: return "agg.pp_stddev_std"   // osu!
		case 1: return "agg.pp_stddev_taiko" // taiko
		case 2: return "agg.pp_stddev_catch" // catch
		default: return "agg.pp_stddev_classic" // mania and others
		}
	case 1: // relax
		return "agg.pp_stddev_relax"
	case 2: // autopilot
		return "agg.pp_stddev_all_modes" // autopilot uses general aggregate
	default:
		return "agg.pp_stddev_all_modes"
	}
}

func applyPPTotal(chosenMode *modeData, modeInt, rx int, ppStd, ppStdRx, ppStdAp, ppTaiko, ppTaikoRx, ppCatch, ppCatchRx, ppMania int) {
	// Apply chosen PP based on mode/rx
	switch modeInt {
	case 0: // osu!
		switch rx {
		case 1:
			chosenMode.PPTotal = ppStdRx  // relax osu!
		case 2:
			chosenMode.PPTotal = ppStdAp  // autopilot osu!
		default:
			chosenMode.PPTotal = ppStd    // vanilla osu!
		}
	case 1: // taiko
		if rx == 1 {
			chosenMode.PPTotal = ppTaikoRx // relax taiko
		} else {
			chosenMode.PPTotal = ppTaiko   // vanilla taiko
		}
	case 2: // catch
		if rx == 1 {
			chosenMode.PPTotal = ppCatchRx // relax catch
		} else {
			chosenMode.PPTotal = ppCatch   // vanilla catch
		}
	case 3: // mania
		chosenMode.PPTotal = ppMania      // mania
	}
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
