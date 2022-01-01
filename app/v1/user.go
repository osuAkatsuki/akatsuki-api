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

const userFields = `SELECT id, name, creation_time, priv,
	latest_activity, name_aka,
	users_stats.country
FROM users
`

func modeConvert(mode, rx int) int {
	if mode == 3 { return 3 }

	if rx == 1 {
		return mode + 4
	} else if rx == 2 {
		return 7
	}

	return mode
}

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
		Where("users.safe_name = ?", common.SafeUsername(md.Query("nname"))).
		Where("users.id = ?", md.Query("iid")).
		Where("users.priv = ?", md.Query("privileges")).
		Where("users.priv & ? > 0", md.Query("has_privileges")).
		Where("users.priv & ? = 0", md.Query("has_not_privileges")).
		Where("users.country = ?", md.Query("country")).
		Where("users.username_aka = ?", md.Query("name_aka")).
		In("users.id", pm("ids")...).
		In("users.safe_name", safeUsernameBulk(pm("names"))...).
		In("users.username_aka", pm("names_aka")...).
		In("users.country", pm("countries")...)

	var extraJoin string

	query := userFields + extraJoin + wh.ClauseSafe() + " AND " + md.User.OnlyUserPublic(true) +
		" " + common.Sort(md, common.SortConfiguration{
		Allowed: []string{
			"id",
			"name",
			"priv",
			"donor_end",
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
	err := md.DB.QueryRow("SELECT id, priv FROM users WHERE safe_name = ? LIMIT 1", common.SafeUsername(md.Query("name"))).Scan(&r.ID, &privileges)
	if err != nil || ((privileges & uint64(common.NORMAL)) == 0 &&
		(md.User.UserPrivileges & common.ADMINISTRATOR == 0)) {
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
	Total_PlayTime         int     `json:"total_playtime"`
	ReplaysWatched         int     `json:"replays_watched"`
	TotalHits              int     `json:"total_hits"`
	Level                  float64 `json:"level"`
	Accuracy               float64 `json:"accuracy"`
	PP                     int     `json:"pp"`
	GlobalLeaderboardRank  *int    `json:"global_leaderboard_rank"`
	CountryLeaderboardRank *int    `json:"country_leaderboard_rank"`
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

	// Fuck.
	r := userFullResponse{}
	var (
		b singleBadge

		can  bool
	)

	query := `
		SELECT id, name, creation_time, priv, latest_activity, username_aka, country, play_style, preferred_mode,
		custom_badge_icon, custom_badge_name,
		silence_end,
		notes, email, clan_id

		WHERE ` + whereClause + ` AND ` + md.User.OnlyUserPublic(true) + `
		LIMIT 1
	`

	err := md.DB.QueryRow(query, param).Scan(
		&r.ID, &r.Username, &r.RegisteredOn, &r.Privileges, &r.LatestActivity,

		&r.UsernameAKA, &r.Country,
		&r.PlayStyle, &r.FavouriteMode,

		&b.Icon, &b.Name,

		&r.SilenceInfo.End,
		&r.CMNotes, &r.Email, &r.Clan.ID,
	)

	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "That user could not be found!")
	case err != nil:
		md.Err(err)
		return Err500
	}

	mode_index := 0
	for i := range []int{0, 1, 2, 3, 4, 5, 6, 7} {
		var index int

		if i <= 3 {
			index = 0
		} else if i <= 6 {
			index = 1
		} else {
			index = 2
		}

		if mode_index == 4 { mode_index = 0 }

		var stat modeData
		switch mode_index {
		case 0:
			stat = r.Stats[index].STD
		case 1:
			stat = r.Stats[index].Taiko
		case 2:
			stat = r.Stats[index].CTB
		case 3:
			stat = r.Stats[index].Mania
		}

		stat_query := `
			SELECT tscore, pp, plays, playtime, acc, total_hits, replay_views
			FROM stats WHERE mode = ? AND id = ?
		`

		err = md.DB.QueryRow(stat_query, i, r.ID).Scan(
			&stat.TotalScore, &stat.PP, &stat.PlayCount, &stat.PlayTime, &stat.Accuracy,
			&stat.TotalHits, &stat.ReplaysWatched,
		)

		mode_index++
	}

	can = common.Privileges(r.Privileges) & common.DONATOR > 0
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
		// I'm sorry for this horribleness but ripple and past mistakes have forced my hand v2
		for modeID, m := range [...]*modeData{&r.Stats[2].STD, &r.Stats[2].Taiko, &r.Stats[2].CTB, &r.Stats[2].Mania} {
			m.Level = ocl.GetLevelPrecise(int64(m.TotalScore))

			if i := autoboardPosition(md.R, modesToReadable[modeID], r.ID); i != nil {
				m.GlobalLeaderboardRank = i
			}
			if i := apcountryPosition(md.R, modesToReadable[modeID], r.ID, r.Country); i != nil {
				m.CountryLeaderboardRank = i
			}
		}

	var follower int
	rows, err := md.DB.Query("SELECT COUNT(id) FROM `relationships` WHERE user2 = ?", r.ID)
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

	if md.User.TokenPrivileges & common.ADMINISTRATOR== 0 {
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
	Userpage *string `json:"userpage"`
}

// UserUserpageGET gets an user's userpage, as in the customisable thing.
func UserUserpageGET(md common.MethodData) common.CodeMessager {
	shouldRet, whereClause, param := whereClauseUser(md, "users")
	if shouldRet != nil {
		return *shouldRet
	}
	var r userpageResponse
	err := md.DB.QueryRow("SELECT userpage_content FROM users WHERE "+whereClause+" LIMIT 1", param).Scan(&r.Userpage)
	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "No such user!")
	case err != nil:
		md.Err(err)
		return Err500
	}
	if r.Userpage == nil {
		r.Userpage = new(string)
	}
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
	_, err := md.DB.Exec("UPDATE users SET userpage_content = ? WHERE id = ? LIMIT 1", cont, md.ID())
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
	rx := common.Int(md.Query("rx"))

	rx_mode := modeConvert(mode, rx)

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
	err := md.DB.QueryRow("SELECT SUM(pp) FROM scores WHERE userid = ? AND completed = 3 AND mode = ?", id, rx_mode).Scan(&r.performance)
	if err != nil {
		if err == sql.ErrNoRows {
			fmt.Println("User", id, "has no scores???")
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
	if md.User.TokenPrivileges & common.ADMINISTRATOR != 0 &&
		strings.Contains(md.Query("name"), "@") {
		email = md.Query("name")
	}

	rows, err := md.DB.Query("SELECT users.id, users.name FROM users WHERE "+
		"(safe_name LIKE ? OR email = ?) AND "+
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
	user := common.Int(md.Query("u"))
	if user == 0 {
		return common.SimpleResponse(401, "Invalid user id!")
	}

	type BeatmapPlaycount struct {
		Count   int     `json:"playcount"`
		Beatmap beatmap `json:"beatmap"`
	}

	type MostPlayedBeatmaps struct {
		common.ResponseBase
		BeatmapsPlaycount []BeatmapPlaycount `json:"most_played_beatmaps"`
	}

	rx_mode := modeConvert(common.Int(md.Query("m")), common.Int(md.Query("rx")))

	// i will query some additional info about the beatmap for later?
	rows, err := md.DB.Query(
		`SELECT COUNT(*) plays,
		maps.id, maps.set_id, maps.md5,
		maps.title, maps.status FROM scores
		INNER JOIN maps ON maps.md5 = scores.map_md5
		WHERE userid = ? AND scores.mode = ? GROUP BY scores.map_md5 ORDER BY count DESC`, user, rx_mode)
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

	return r
}
