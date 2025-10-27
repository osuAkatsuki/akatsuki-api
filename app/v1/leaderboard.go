package v1

import (
	"fmt"
	"strconv"
	"strings"

	"database/sql"

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

// LeaderboardGET gets the leaderboard.
func LeaderboardGET(md common.MethodData) common.CodeMessager {
	m := getMode(md.Query("mode"))
	modeInt := getModeInt(m)

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

	// Для стандартных сортировок (pp/score) используем Redis + точные позиции
	if sort == "pp" || sort == "score" {
		return getStandardLeaderboard(m, modeInt, p, l, rx, sort, md)
	}

	// Для clean сортировок (total/spp) - прямой SQL запрос с пагинацией
	return getAggregateLeaderboard(m, modeInt, p, l, rx, sort, md)
}

func getStandardLeaderboard(m string, modeInt, p, l, rx int, sort string, md common.MethodData) common.CodeMessager {
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

	var sqlOrder string
	if sort == "score" {
		sqlOrder = "ORDER BY user_stats.ranked_score DESC, user_stats.pp DESC"
	} else {
		sqlOrder = "ORDER BY user_stats.pp DESC, user_stats.ranked_score DESC"
	}

	var query = lbUserQuery + ` WHERE users.id IN (?) AND user_stats.mode = ? ` + sqlOrder
	query, params, _ := sqlx.In(query, results, modeInt+(rx*4))
	rows, err := md.DB.Query(query, params...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()

	return scanLeaderboardRows(rows, m, rx, md, &resp)
}

func getAggregateLeaderboard(m string, modeInt, p, l, rx int, sort string, md common.MethodData) common.CodeMessager {
	var order string
	
	// Clean подход: простые sort=total и sort=spp
	switch sort {
	case "total":
		order = getTotalOrder(modeInt, rx)
	case "spp":
		order = getStdDevOrder(modeInt, rx)
	default:
		// Fallback на стандартную сортировку
		order = "ORDER BY user_stats.pp DESC, users.id ASC"
	}

	query := fmt.Sprintf(lbUserQuery+` 
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

	// Для aggregate сортировок позиции = номер строки + смещение страницы
	globalRank := p*l + 1
	return scanLeaderboardRowsWithOffset(rows, m, rx, md, &resp, globalRank)
}

func getTotalOrder(modeInt, rx int) string {
	// Определяем поле для Total PP на основе mode и rx
	if modeInt >= 0 && rx >= 0 {
		// Конкретный режим + конкретный rx
		return fmt.Sprintf("ORDER BY %s DESC, users.id ASC", getIndividualPPField(modeInt, rx))
	} else if modeInt >= 0 {
		// Конкретный режим, все rx (std+rx+ap для osu!)
		switch modeInt {
		case 0: return "ORDER BY agg.pp_total_std DESC, users.id ASC"      // osu! все rx
		case 1: return "ORDER BY agg.pp_total_taiko DESC, users.id ASC"    // taiko все rx  
		case 2: return "ORDER BY agg.pp_total_catch DESC, users.id ASC"    // catch все rx
		case 3: return "ORDER BY agg.pp_mania DESC, users.id ASC"          // mania
		default: return "ORDER BY agg.pp_total_all_modes DESC, users.id ASC"
		}
	} else if rx >= 0 {
		// Все режимы для конкретного rx
		switch rx {
		case 0: return "ORDER BY agg.pp_total_classic DESC, users.id ASC"      // vanilla
		case 1: return "ORDER BY agg.pp_total_relax DESC, users.id ASC"        // relax
		case 2: return "ORDER BY agg.pp_total_all_modes DESC, users.id ASC"    // autopilot
		default: return "ORDER BY agg.pp_total_all_modes DESC, users.id ASC"
		}
	} else {
		// Всего вообще (без фильтров)
		return "ORDER BY agg.pp_total_all_modes DESC, users.id ASC"
	}
}

func getStdDevOrder(modeInt, rx int) string {
	// Определяем поле для SPP на основе mode и rx
	if modeInt >= 0 && rx >= 0 {
		// Конкретный режим + конкретный rx
		return fmt.Sprintf("ORDER BY %s DESC, users.id ASC", getIndividualStdDevField(modeInt, rx))
	} else if modeInt >= 0 {
		// Конкретный режим, все rx
		switch modeInt {
		case 0: return "ORDER BY agg.pp_stddev_std DESC, users.id ASC"     // osu! все rx
		case 1: return "ORDER BY agg.pp_stddev_taiko DESC, users.id ASC"   // taiko все rx
		case 2: return "ORDER BY agg.pp_stddev_catch DESC, users.id ASC"   // catch все rx
		default: return "ORDER BY agg.pp_stddev_classic DESC, users.id ASC" // mania
		}
	} else if rx >= 0 {
		// Все режимы для конкретного rx
		switch rx {
		case 0: return "ORDER BY agg.pp_stddev_classic DESC, users.id ASC"     // vanilla
		case 1: return "ORDER BY agg.pp_stddev_relax DESC, users.id ASC"       // relax
		case 2: return "ORDER BY agg.pp_stddev_all_modes DESC, users.id ASC"   // autopilot
		default: return "ORDER BY agg.pp_stddev_all_modes DESC, users.id ASC"
		}
	} else {
		// Всего вообще (без фильтров)
		return "ORDER BY agg.pp_stddev_all_modes DESC, users.id ASC"
	}
}

func getIndividualPPField(modeInt, rx int) string {
	// Individual PP поля для конкретного режима и rx
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
		return "agg.pp_mania" // только vanilla
	}
	return "user_stats.pp" // fallback
}

func getIndividualStdDevField(modeInt, rx int) string {
	// Individual StdDev поля для конкретного режима и rx
	switch rx {
	case 0: // vanilla
		switch modeInt {
		case 0: return "agg.pp_stddev_std"   // osu!
		case 1: return "agg.pp_stddev_taiko" // taiko
		case 2: return "agg.pp_stddev_catch" // catch
		default: return "agg.pp_stddev_classic" // mania и другие
		}
	case 1: // relax
		return "agg.pp_stddev_relax"
	case 2: // autopilot
		return "agg.pp_stddev_all_modes" // autopilot использует общий агрегат
	default:
		return "agg.pp_stddev_all_modes"
	}
}

func scanLeaderboardRows(rows *sql.Rows, m string, rx int, md common.MethodData, resp *leaderboardResponse) common.CodeMessager {
	for rows.Next() {
		userDB, chosenMode, err := scanUserRow(rows)
		if err != nil {
			md.Err(err)
			continue
		}

		applyPPTotal(&chosenMode, userDB.FavouriteMode, rx)

		chosenMode.Level = ocl.GetLevelPrecise(int64(chosenMode.TotalScore))

		eligibleTitles, err := getEligibleTitles(md, userDB.ID, userDB.Privileges)
		if err != nil {
			md.Err(err)
			return Err500
		}

		u := userDB.toLeaderboardUser(eligibleTitles)
		u.ChosenMode = chosenMode

		setLeaderboardPositions(&u, m, rx, md.R)

		resp.Users = append(resp.Users, u)
	}
	return resp
}

func scanLeaderboardRowsWithOffset(rows *sql.Rows, m string, rx int, md common.MethodData, resp *leaderboardResponse, startRank int) common.CodeMessager {
	currentRank := startRank
	for rows.Next() {
		userDB, chosenMode, err := scanUserRow(rows)
		if err != nil {
			md.Err(err)
			continue
		}

		applyPPTotal(&chosenMode, userDB.FavouriteMode, rx)

		chosenMode.Level = ocl.GetLevelPrecise(int64(chosenMode.TotalScore))

		eligibleTitles, err := getEligibleTitles(md, userDB.ID, userDB.Privileges)
		if err != nil {
			md.Err(err)
			return Err500
		}

		u := userDB.toLeaderboardUser(eligibleTitles)
		u.ChosenMode = chosenMode

		// Для aggregate сортировок используем вычисленные позиции
		globalRank := currentRank
		u.ChosenMode.GlobalLeaderboardRank = &globalRank
		
		// Страновые позиции не поддерживаются для aggregate сортировок
		currentRank++

		resp.Users = append(resp.Users, u)
	}
	return resp
}

func scanUserRow(rows *sql.Rows) (leaderboardUserDB, modeData, error) {
	var userDB leaderboardUserDB
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

	return userDB, chosenMode, err
}

func applyPPTotal(chosenMode *modeData, modeInt, rx int) {
	// Apply chosen PP based on mode/rx
	switch modeInt {
	case 0: // osu!
		switch rx {
		case 1:
			chosenMode.PPTotal = chosenMode.PP // Используем pp_std_rx если доступно
		case 2:
			chosenMode.PPTotal = chosenMode.PP // Используем pp_std_ap если доступно
		default:
			chosenMode.PPTotal = chosenMode.PP // Используем pp_std если доступно
		}
	case 1: // taiko
		if rx == 1 {
			chosenMode.PPTotal = chosenMode.PP // Используем pp_taiko_rx если доступно
		} else {
			chosenMode.PPTotal = chosenMode.PP // Используем pp_taiko если доступно
		}
	case 2: // catch
		if rx == 1 {
			chosenMode.PPTotal = chosenMode.PP // Используем pp_catch_rx если доступно
		} else {
			chosenMode.PPTotal = chosenMode.PP // Используем pp_catch если доступно
		}
	case 3: // mania
		chosenMode.PPTotal = chosenMode.PP // Используем pp_mania если доступно
	}
}

func setLeaderboardPositions(u *leaderboardUser, m string, rx int, r *redis.Client) {
	if rx == 1 {
		if i := relaxboardPosition(r, m, u.ID); i != nil {
			u.ChosenMode.GlobalLeaderboardRank = i
		}
		if i := rxcountryPosition(r, m, u.ID, u.Country); i != nil {
			u.ChosenMode.CountryLeaderboardRank = i
		}
	} else if rx == 2 {
		if i := autoboardPosition(r, m, u.ID); i != nil {
			u.ChosenMode.GlobalLeaderboardRank = i
		}
		if i := apcountryPosition(r, m, u.ID, u.Country); i != nil {
			u.ChosenMode.CountryLeaderboardRank = i
		}
	} else {
		if i := leaderboardPosition(r, m, u.ID); i != nil {
			u.ChosenMode.GlobalLeaderboardRank = i
		}
		if i := countryPosition(r, m, u.ID, u.Country); i != nil {
			u.ChosenMode.CountryLeaderboardRank = i
		}
	}
}

// Redis position functions
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
