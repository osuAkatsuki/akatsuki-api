// Package v1 implements the first version of the Ripple API.
package v1

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/jmoiron/sqlx"
	"github.com/osuAkatsuki/akatsuki-api/common"
	"github.com/osuAkatsuki/akatsuki-api/externals"
	"zxq.co/ripple/ocl"
)

type userData struct {
	ID             int                  `json:"id"`
	Username       string               `json:"username"`
	UsernameAKA    string               `json:"username_aka"`
	RegisteredOn   common.UnixTimestamp `json:"registered_on"`
	Privileges     uint64               `json:"privileges"`
	LatestActivity common.UnixTimestamp `json:"latest_activity"`
	Country        string               `json:"country"`
}

const userFields = `SELECT users.id, users.username, register_datetime, users.privileges,
	latest_activity, users_stats.username_aka,
	users_stats.country
FROM users
INNER JOIN users_stats
ON users.id=users_stats.id
`

// UsersGET is the API handler for GET /users
func UsersGET(md common.MethodData) common.CodeMessager {
	shouldRet, whereClause, param := whereClauseUser(md, "users")
	if shouldRet != nil {
		return userPutsMulti(md)
	}

	query := userFields + `
WHERE ` + whereClause + ` AND ` + md.User.OnlyUserPublic(true) + `
LIMIT 1`
	return userPutsSingle(md, md.DB.QueryRowx(query, param))
}

type userPutsSingleUserData struct {
	common.ResponseBase
	userData
}

func userPutsSingle(md common.MethodData, row *sqlx.Row) common.CodeMessager {
	var err error
	var user userPutsSingleUserData

	err = row.StructScan(&user.userData)
	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "No such user was found!")
	case err != nil:
		md.Err(err)
		return Err500
	}

	user.Code = 200
	return user
}

type userPutsMultiUserData struct {
	common.ResponseBase
	Users []userData `json:"users"`
}

func userPutsMulti(md common.MethodData) common.CodeMessager {
	pm := md.Ctx.Request.URI().QueryArgs().PeekMulti
	// query composition
	wh := common.
		Where("users.username_safe = ?", common.SafeUsername(md.Query("nname"))).
		Where("users.id = ?", md.Query("iid")).
		Where("users.privileges = ?", md.Query("privileges")).
		Where("users.privileges & ? > 0", md.Query("has_privileges")).
		Where("users.privileges & ? = 0", md.Query("has_not_privileges")).
		Where("users_stats.country = ?", md.Query("country")).
		Where("users_stats.username_aka = ?", md.Query("name_aka")).
		Where("privileges_groups.name = ?", md.Query("privilege_group")).
		In("users.id", pm("ids")...).
		In("users.username_safe", safeUsernameBulk(pm("names"))...).
		In("users_stats.username_aka", pm("names_aka")...).
		In("users_stats.country", pm("countries")...)

	var extraJoin string
	if md.Query("privilege_group") != "" {
		extraJoin = " LEFT JOIN privileges_groups ON users.privileges & privileges_groups.privileges = privileges_groups.privileges "
	}

	query := userFields + extraJoin + wh.ClauseSafe() + " AND " + md.User.OnlyUserPublic(true) +
		" " + common.Sort(md, common.SortConfiguration{
		Allowed: []string{
			"id",
			"username",
			"privileges",
			"donor_expire",
			"latest_activity",
			"silence_end",
		},
		Default: "id ASC",
		Table:   "users",
	}) +
		" " + common.Paginate(md.Query("p"), md.Query("l"), 100)

	// query execution
	rows, err := md.DB.Queryx(query, wh.Params...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	var r userPutsMultiUserData
	for rows.Next() {
		var u userData
		err := rows.StructScan(&u)
		if err != nil {
			md.Err(err)
			continue
		}
		r.Users = append(r.Users, u)
	}
	r.Code = 200
	return r
}

// UserSelfGET is a shortcut for /users/id/self. (/users/self)
func UserSelfGET(md common.MethodData) common.CodeMessager {
	md.Ctx.Request.URI().SetQueryString("id=self")
	return UsersGET(md)
}

func safeUsernameBulk(us [][]byte) [][]byte {
	for _, u := range us {
		for idx, v := range u {
			if v == ' ' {
				u[idx] = '_'
				continue
			}
			u[idx] = byte(unicode.ToLower(rune(v)))
		}
	}
	return us
}

type whatIDResponse struct {
	common.ResponseBase
	ID int `json:"id"`
}

// UserWhatsTheIDGET is an API request that only returns an user's ID.
func UserWhatsTheIDGET(md common.MethodData) common.CodeMessager {
	var (
		r          whatIDResponse
		privileges uint64
	)
	err := md.DB.QueryRow("SELECT id, privileges FROM users WHERE username_safe = ? LIMIT 1", common.SafeUsername(md.Query("name"))).Scan(&r.ID, &privileges)
	if err != nil || ((privileges&uint64(common.UserPrivilegePublic)) == 0 &&
		(md.User.UserPrivileges&common.AdminPrivilegeManageUsers == 0)) {
		return common.SimpleResponse(404, "That user could not be found!")
	}
	r.Code = 200
	return r
}

var modesToReadable = [...]string{
	"std",
	"taiko",
	"ctb",
	"mania",
}

type modeData struct {
	RankedScore            uint64  `json:"ranked_score"`
	TotalScore             uint64  `json:"total_score"`
	PlayCount              int     `json:"playcount"`
	PlayTime               int     `json:"playtime"`
	ReplaysWatched         int     `json:"replays_watched"`
	TotalHits              int     `json:"total_hits"`
	Level                  float64 `json:"level"`
	Accuracy               float64 `json:"accuracy"`
	PP                     int     `json:"pp"`
	GlobalLeaderboardRank  *int    `json:"global_leaderboard_rank"`
	CountryLeaderboardRank *int    `json:"country_leaderboard_rank"`
	MaxCombo               int     `json:"max_combo"`
}

type userStats struct {
	STD   modeData `json:"std"`
	Taiko modeData `json:"taiko"`
	CTB   modeData `json:"ctb"`
	Mania modeData `json:"mania"`
}

type userFullResponse struct {
	common.ResponseBase
	userData
	Stats         [3]userStats          `json:"stats"`
	PlayStyle     int                   `json:"play_style"`
	FavouriteMode int                   `json:"favourite_mode"`
	Badges        []singleBadge         `json:"badges"`
	Clan          Clan                  `json:"clan"`
	Followers     int                   `json:"followers"`
	TBadges       []TsingleBadge        `json:"tbadges"`
	CustomBadge   *singleBadge          `json:"custom_badge"`
	SilenceInfo   silenceInfo           `json:"silence_info"`
	CMNotes       *string               `json:"cm_notes,omitempty"`
	BanDate       *common.UnixTimestamp `json:"ban_date,omitempty"`
	Email         string                `json:"email,omitempty"`
}
type silenceInfo struct {
	Reason string               `json:"reason"`
	End    common.UnixTimestamp `json:"end"`
}

// UserFullGET gets all of an user's information, with one exception: their userpage.
func UserFullGET(md common.MethodData) common.CodeMessager {
	shouldRet, whereClause, param := whereClauseUser(md, "users")
	if shouldRet != nil {
		return *shouldRet
	}

	// Hellest query I've ever done.
	query := `
SELECT
	users.id, users.username, users.register_datetime, users.privileges, users.latest_activity,

	users_stats.username_aka, users_stats.country, users_stats.play_style, users_stats.favourite_mode,

	users_stats.custom_badge_icon, users_stats.custom_badge_name, users_stats.can_custom_badge,
	users_stats.show_custom_badge,

	users_stats.ranked_score_std, users_stats.total_score_std, users_stats.playcount_std, users_stats.playtime_std,
	users_stats.replays_watched_std, users_stats.total_hits_std,
	users_stats.avg_accuracy_std, users_stats.pp_std, users_stats.max_combo_std,

	users_stats.ranked_score_taiko, users_stats.total_score_taiko, users_stats.playcount_taiko, users_stats.playtime_taiko,
	users_stats.replays_watched_taiko, users_stats.total_hits_taiko,
	users_stats.avg_accuracy_taiko, users_stats.pp_taiko, users_stats.max_combo_taiko,

	users_stats.ranked_score_ctb, users_stats.total_score_ctb, users_stats.playcount_ctb, users_stats.playtime_ctb,
	users_stats.replays_watched_ctb, users_stats.total_hits_ctb,
	users_stats.avg_accuracy_ctb, users_stats.pp_ctb, users_stats.max_combo_ctb,

	users_stats.ranked_score_mania, users_stats.total_score_mania, users_stats.playcount_mania, users_stats.playtime_mania,
	users_stats.replays_watched_mania, users_stats.total_hits_mania,
	users_stats.avg_accuracy_mania, users_stats.pp_mania, users_stats.max_combo_mania,
	
	rx_stats.ranked_score_std, rx_stats.total_score_std, rx_stats.playcount_std, users_stats.playtime_std,
	rx_stats.replays_watched_std, rx_stats.total_hits_std,
	rx_stats.avg_accuracy_std, rx_stats.pp_std, rx_stats.max_combo_std,

	rx_stats.ranked_score_taiko, rx_stats.total_score_taiko, rx_stats.playcount_taiko, users_stats.playtime_taiko,
	rx_stats.replays_watched_taiko, rx_stats.total_hits_taiko,
	rx_stats.avg_accuracy_taiko, rx_stats.pp_taiko, rx_stats.max_combo_taiko,

	rx_stats.ranked_score_ctb, rx_stats.total_score_ctb, rx_stats.playcount_ctb, users_stats.playtime_ctb,
	rx_stats.replays_watched_ctb, rx_stats.total_hits_ctb,
	rx_stats.avg_accuracy_ctb, rx_stats.pp_ctb, rx_stats.max_combo_ctb,

	rx_stats.ranked_score_mania, rx_stats.total_score_mania, rx_stats.playcount_mania, users_stats.playtime_mania,
	rx_stats.replays_watched_mania, rx_stats.total_hits_mania,
	rx_stats.avg_accuracy_mania, rx_stats.pp_mania, rx_stats.max_combo_mania,

	ap_stats.ranked_score_std, ap_stats.total_score_std, ap_stats.playcount_std, users_stats.playtime_std,
	ap_stats.replays_watched_std, ap_stats.total_hits_std,
	ap_stats.avg_accuracy_std, ap_stats.pp_std, ap_stats.max_combo_std,

	users.silence_reason, users.silence_end,
	users.notes, users.ban_datetime, users.email,
	users.clan_id

FROM users
LEFT JOIN users_stats
ON users.id=users_stats.id
LEFT JOIN rx_stats
ON users.id=rx_stats.id
LEFT JOIN ap_stats
ON users.id=ap_stats.id
WHERE ` + whereClause + ` AND ` + md.User.OnlyUserPublic(true) + `
LIMIT 1
`
	// Fuck.
	r := userFullResponse{}
	var (
		b singleBadge

		can  bool
		show bool
	)
	err := md.DB.QueryRow(query, param).Scan(
		&r.ID, &r.Username, &r.RegisteredOn, &r.Privileges, &r.LatestActivity,

		&r.UsernameAKA, &r.Country,
		&r.PlayStyle, &r.FavouriteMode,

		&b.Icon, &b.Name, &can, &show,

		&r.Stats[0].STD.RankedScore, &r.Stats[0].STD.TotalScore, &r.Stats[0].STD.PlayCount, &r.Stats[0].STD.PlayTime,
		&r.Stats[0].STD.ReplaysWatched, &r.Stats[0].STD.TotalHits,
		&r.Stats[0].STD.Accuracy, &r.Stats[0].STD.PP, &r.Stats[0].STD.MaxCombo,

		&r.Stats[0].Taiko.RankedScore, &r.Stats[0].Taiko.TotalScore, &r.Stats[0].Taiko.PlayCount, &r.Stats[0].Taiko.PlayTime,
		&r.Stats[0].Taiko.ReplaysWatched, &r.Stats[0].Taiko.TotalHits,
		&r.Stats[0].Taiko.Accuracy, &r.Stats[0].Taiko.PP, &r.Stats[0].Taiko.MaxCombo,

		&r.Stats[0].CTB.RankedScore, &r.Stats[0].CTB.TotalScore, &r.Stats[0].CTB.PlayCount, &r.Stats[0].CTB.PlayTime,
		&r.Stats[0].CTB.ReplaysWatched, &r.Stats[0].CTB.TotalHits,
		&r.Stats[0].CTB.Accuracy, &r.Stats[0].CTB.PP, &r.Stats[0].CTB.MaxCombo,

		&r.Stats[0].Mania.RankedScore, &r.Stats[0].Mania.TotalScore, &r.Stats[0].Mania.PlayCount, &r.Stats[0].Mania.PlayTime,
		&r.Stats[0].Mania.ReplaysWatched, &r.Stats[0].Mania.TotalHits,
		&r.Stats[0].Mania.Accuracy, &r.Stats[0].Mania.PP, &r.Stats[0].Mania.MaxCombo,

		&r.Stats[1].STD.RankedScore, &r.Stats[1].STD.TotalScore, &r.Stats[1].STD.PlayCount, &r.Stats[1].STD.PlayTime,
		&r.Stats[1].STD.ReplaysWatched, &r.Stats[1].STD.TotalHits,
		&r.Stats[1].STD.Accuracy, &r.Stats[1].STD.PP, &r.Stats[1].STD.MaxCombo,

		&r.Stats[1].Taiko.RankedScore, &r.Stats[1].Taiko.TotalScore, &r.Stats[1].Taiko.PlayCount, &r.Stats[1].Taiko.PlayTime,
		&r.Stats[1].Taiko.ReplaysWatched, &r.Stats[1].Taiko.TotalHits,
		&r.Stats[1].Taiko.Accuracy, &r.Stats[1].Taiko.PP, &r.Stats[1].Taiko.MaxCombo,

		&r.Stats[1].CTB.RankedScore, &r.Stats[1].CTB.TotalScore, &r.Stats[1].CTB.PlayCount, &r.Stats[1].CTB.PlayTime,
		&r.Stats[1].CTB.ReplaysWatched, &r.Stats[1].CTB.TotalHits,
		&r.Stats[1].CTB.Accuracy, &r.Stats[1].CTB.PP, &r.Stats[1].CTB.MaxCombo,

		&r.Stats[1].Mania.RankedScore, &r.Stats[1].Mania.TotalScore, &r.Stats[1].Mania.PlayCount, &r.Stats[1].Mania.PlayTime,
		&r.Stats[1].Mania.ReplaysWatched, &r.Stats[1].Mania.TotalHits,
		&r.Stats[1].Mania.Accuracy, &r.Stats[1].Mania.PP, &r.Stats[1].Mania.MaxCombo,

		&r.Stats[2].STD.RankedScore, &r.Stats[2].STD.TotalScore, &r.Stats[2].STD.PlayCount, &r.Stats[2].STD.PlayTime,
		&r.Stats[2].STD.ReplaysWatched, &r.Stats[2].STD.TotalHits,
		&r.Stats[2].STD.Accuracy, &r.Stats[2].STD.PP, &r.Stats[2].STD.MaxCombo,

		&r.SilenceInfo.Reason, &r.SilenceInfo.End,
		&r.CMNotes, &r.BanDate, &r.Email, &r.Clan.ID,
	)
	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "That user could not be found!")
	case err != nil:
		md.Err(err)
		return Err500
	}

	can = can && show && common.UserPrivileges(r.Privileges)&common.UserPrivilegeDonor > 0
	if can && (b.Name != "" || b.Icon != "") {
		r.CustomBadge = &b
	}

	for modeID, m := range [...]*modeData{&r.Stats[0].STD, &r.Stats[0].Taiko, &r.Stats[0].CTB, &r.Stats[0].Mania} {
		m.Level = ocl.GetLevelPrecise(int64(m.TotalScore))

		if i := leaderboardPosition(md.R, modesToReadable[modeID], r.ID); i != nil {
			m.GlobalLeaderboardRank = i
		}
		if i := countryPosition(md.R, modesToReadable[modeID], r.ID, r.Country); i != nil {
			m.CountryLeaderboardRank = i
		}
	}
	// I'm sorry for this horribleness but ripple and past mistakes have forced my hand
	for modeID, m := range [...]*modeData{&r.Stats[1].STD, &r.Stats[1].Taiko, &r.Stats[1].CTB, &r.Stats[1].Mania} {
		m.Level = ocl.GetLevelPrecise(int64(m.TotalScore))

		if i := relaxboardPosition(md.R, modesToReadable[modeID], r.ID); i != nil {
			m.GlobalLeaderboardRank = i
		}
		if i := rxcountryPosition(md.R, modesToReadable[modeID], r.ID, r.Country); i != nil {
			m.CountryLeaderboardRank = i
		}
	}

	for modeID, m := range [...]*modeData{&r.Stats[2].STD} {
		m.Level = ocl.GetLevelPrecise(int64(m.TotalScore))

		if i := autoboardPosition(md.R, modesToReadable[modeID], r.ID); i != nil {
			m.GlobalLeaderboardRank = i
		}
		if i := apcountryPosition(md.R, modesToReadable[modeID], r.ID, r.Country); i != nil {
			m.CountryLeaderboardRank = i
		}
	}

	var follower int
	rows, err := md.DB.Query("SELECT COUNT(id) FROM `users_relationships` WHERE user2 = ?", r.ID)
	if err != nil {
		md.Err(err)
	}
	for rows.Next() {
		err := rows.Scan(&follower)
		if err != nil {
			md.Err(err)
			continue
		}
	}
	r.Followers = follower

	rows, err = md.DB.Query("SELECT b.id, b.name, b.icon FROM user_badges ub "+
		"LEFT JOIN badges b ON ub.badge = b.id WHERE user = ?", r.ID)
	if err != nil {
		md.Err(err)
	}

	for rows.Next() {
		var badge singleBadge
		err := rows.Scan(&badge.ID, &badge.Name, &badge.Icon)
		if err != nil {
			md.Err(err)
			continue
		}
		r.Badges = append(r.Badges, badge)
	}

	if md.User.TokenPrivileges&common.PrivilegeManageUser == 0 {
		r.CMNotes = nil
		r.BanDate = nil
		r.Email = ""
	}

	rows, err = md.DB.Query("SELECT tb.id, tb.name, tb.icon FROM user_tourmnt_badges tub "+
		"LEFT JOIN tourmnt_badges tb ON tub.badge = tb.id WHERE user = ?", r.ID)
	if err != nil {
		md.Err(err)
	}

	for rows.Next() {
		var Tbadge TsingleBadge
		err := rows.Scan(&Tbadge.ID, &Tbadge.Name, &Tbadge.Icon)
		if err != nil {
			md.Err(err)
			continue
		}
		r.TBadges = append(r.TBadges, Tbadge)
	}

	r.Clan, err = getClan(r.Clan.ID, md)
	if err != nil {
		md.Err(err)
	}

	r.Code = 200
	return r
}

type userpageResponse struct {
	common.ResponseBase
	Userpage         *string `json:"userpage"`
	UserpageCompiled string  `json:"userpage_compiled"`
}

// UserUserpageGET gets an user's userpage, as in the customisable thing.
func UserUserpageGET(md common.MethodData) common.CodeMessager {
	shouldRet, whereClause, param := whereClauseUser(md, "users_stats")
	if shouldRet != nil {
		return *shouldRet
	}
	var r userpageResponse
	err := md.DB.QueryRow("SELECT userpage_content FROM users_stats WHERE "+whereClause+" LIMIT 1", param).Scan(&r.Userpage)
	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "No such user!")
	case err != nil:
		md.Err(err)
		return Err500
	}
	if r.Userpage == nil {
		r.Userpage = new(string)
		r.UserpageCompiled = ""
	}
	r.UserpageCompiled = externals.ConvertBBCodeToHTML(*r.Userpage)
	r.Code = 200
	return r
}

// UserSelfUserpagePOST allows to change the current user's userpage.
func UserSelfUserpagePOST(md common.MethodData) common.CodeMessager {
	var d struct {
		Data *string `json:"data"`
	}
	md.Unmarshal(&d)
	if d.Data == nil {
		return ErrMissingField("data")
	}
	cont := common.SanitiseString(*d.Data)
	_, err := md.DB.Exec("UPDATE users_stats SET userpage_content = ? WHERE id = ? LIMIT 1", cont, md.ID())
	if err != nil {
		md.Err(err)
	}
	md.Ctx.URI().SetQueryString("id=self")
	return UserUserpageGET(md)
}

func whereClauseUser(md common.MethodData, tableName string) (*common.CodeMessager, string, interface{}) {
	switch {
	case md.Query("id") == "self":
		return nil, tableName + ".id = ?", md.ID()
	case md.Query("id") != "":
		id, err := strconv.Atoi(md.Query("id"))
		if err != nil {
			a := common.SimpleResponse(400, "please pass a valid user ID")
			return &a, "", nil
		}
		return nil, tableName + ".id = ?", id
	case md.Query("name") != "":
		return nil, tableName + ".username_safe = ?", common.SafeUsername(md.Query("name"))
	}
	a := common.SimpleResponse(400, "you need to pass either querystring parameters name or id")
	return &a, "", nil
}

func UserUnweightedPerformanceGET(md common.MethodData) common.CodeMessager {
	id := common.Int(md.Query("id"))
	if id <= 0 {
		return ErrMissingField("id")
	}
	mode := common.Int(md.Query("mode"))
	tab := ""
	if md.Query("rx") == "1" {
		tab = "_relax"
	} else if md.Query("rx") == "2" {
		tab = "_ap"
	}

	if err := md.DB.QueryRow("SELECT 1 FROM users WHERE id = ?", id).Scan(&id); err == sql.ErrNoRows {
		return common.SimpleResponse(404, "user not found")
	} else if err != nil {
		md.Err(err)
		return Err500
	}

	r := struct {
		common.ResponseBase
		performance float32 `json:"performance"`
	}{}
	err := md.DB.QueryRow("SELECT SUM(pp) FROM scores"+tab+" WHERE userid = ? AND completed = 3 AND mode = ?", id, mode).Scan(&r.performance)
	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Println("User", id, "has no scores in scores"+tab, "???")
			return r
		}
		return Err500
	}

	r.Code = 200
	return r
}

type userLookupResponse struct {
	common.ResponseBase
	Users []lookupUser `json:"users"`
}
type lookupUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

// UserLookupGET does a quick lookup of users beginning with the passed
// querystring value name.
func UserLookupGET(md common.MethodData) common.CodeMessager {
	name := common.SafeUsername(md.Query("name"))
	name = strings.NewReplacer(
		"%", "\\%",
		"_", "\\_",
		"\\", "\\\\",
	).Replace(name)
	if name == "" {
		return common.SimpleResponse(400, "please provide an username to start searching")
	}
	name = "%" + name + "%"

	var email string
	if md.User.TokenPrivileges&common.PrivilegeManageUser != 0 &&
		strings.Contains(md.Query("name"), "@") {
		email = md.Query("name")
	}

	rows, err := md.DB.Query("SELECT users.id, users.username FROM users WHERE "+
		"(username_safe LIKE ? OR email = ?) AND "+
		md.User.OnlyUserPublic(true)+" LIMIT 25", name, email)
	if err != nil {
		md.Err(err)
		return Err500
	}

	var r userLookupResponse
	for rows.Next() {
		var l lookupUser
		err := rows.Scan(&l.ID, &l.Username)
		if err != nil {
			continue // can't be bothered to handle properly
		}
		r.Users = append(r.Users, l)
	}

	r.Code = 200
	return r
}

func UserMostPlayedBeatmapsGET(md common.MethodData) common.CodeMessager {
	user := common.Int(md.Query("id"))
	if user == 0 {
		return common.SimpleResponse(401, "Invalid user id!")
	}

	relax := common.Int(md.Query("rx"))
	mode := common.Int(md.Query("mode"))

	type BeatmapPlaycount struct {
		Count   int     `json:"playcount"`
		Beatmap beatmap `json:"beatmap"`
	}

	type MostPlayedBeatmaps struct {
		common.ResponseBase
		BeatmapsPlaycount []BeatmapPlaycount `json:"most_played_beatmaps"`
	}

	// i will query some additional info about the beatmap for later?
	rows, err := md.DB.Query(
		fmt.Sprintf(
			`SELECT user_beatmaps.count,
		beatmaps.beatmap_id, beatmaps.beatmapset_id, beatmaps.beatmap_md5,
		beatmaps.song_name, beatmaps.ranked FROM user_beatmaps
		INNER JOIN beatmaps ON beatmaps.beatmap_md5 = user_beatmaps.map
		WHERE userid = ? AND rx = ? AND user_beatmaps.mode = ? ORDER BY count DESC %s`, common.Paginate(md.Query("p"), md.Query("l"), 100)),
		user, relax, mode)

	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()

	r := MostPlayedBeatmaps{}

	for rows.Next() {
		bmc := BeatmapPlaycount{}

		err = rows.Scan(&bmc.Count, &bmc.Beatmap.BeatmapID, &bmc.Beatmap.BeatmapsetID,
			&bmc.Beatmap.BeatmapMD5, &bmc.Beatmap.SongName, &bmc.Beatmap.Ranked)
		if err != nil {
			md.Err(err)
			return Err500
		}

		r.BeatmapsPlaycount = append(r.BeatmapsPlaycount, bmc)
	}

	r.Code = 200
	return r
}
