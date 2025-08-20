// Package v1 implements the first version of the Ripple API.
package v1

import (
	"database/sql"
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode"

	"github.com/jmoiron/sqlx"
	"github.com/osuAkatsuki/akatsuki-api/common"
	"github.com/osuAkatsuki/akatsuki-api/externals"
	"golang.org/x/exp/slog"
	"zxq.co/ripple/ocl"
)

// userDataDB is used for scanning from database
type userDataDB struct {
	ID             int                  `db:"id"`
	Username       string               `db:"username"`
	UsernameAKA    string               `db:"username_aka"`
	RegisteredOn   common.UnixTimestamp `db:"register_datetime"`
	Privileges     uint64               `db:"privileges"`
	LatestActivity common.UnixTimestamp `db:"latest_activity"`
	Country        string               `db:"country"`
	UserTitle      sql.NullString       `db:"user_title"`
}

// userData is used for API responses (contains userTitleResponse)
type userData struct {
	ID             int                  `json:"id"`
	Username       string               `json:"username"`
	UsernameAKA    string               `json:"username_aka"`
	RegisteredOn   common.UnixTimestamp `json:"registered_on"`
	Privileges     uint64               `json:"privileges"`
	LatestActivity common.UnixTimestamp `json:"latest_activity"`
	Country        string               `json:"country"`
	UserTitle      userTitleResponse    `json:"user_title"`
}

// toUserData converts userDataDB to userData with proper title conversion
func (udb *userDataDB) toUserData(eligibleTitles []eligibleTitle) userData {
	u := userData{
		ID:             udb.ID,
		Username:       udb.Username,
		UsernameAKA:    udb.UsernameAKA,
		RegisteredOn:   udb.RegisteredOn,
		Privileges:     udb.Privileges,
		LatestActivity: udb.LatestActivity,
		Country:        udb.Country,
	}

	// Convert UserTitle ID to structured response
	if udb.UserTitle.Valid && udb.UserTitle.String != "" {
		u.UserTitle = userTitleResponse{
			ID:    udb.UserTitle.String,
			Title: getUserTitleFromID(udb.UserTitle.String),
		}
	} else {
		u.UserTitle = userTitleResponse{
			ID:    eligibleTitles[0].ID,
			Title: eligibleTitles[0].Title,
		}

	}

	return u
}

const userFields = `
SELECT users.id, users.username, users.register_datetime, users.privileges,
users.latest_activity, users.username_aka, users.country, users.user_title
FROM users
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
	var userDataDB userDataDB

	err = row.StructScan(&userDataDB)
	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "No such user was found!")
	case err != nil:
		md.Err(err)
		return Err500
	}

	var eligibleTitles []eligibleTitle
	eligibleTitles, err = getEligibleTitles(md, userDataDB.Privileges)
	if err != nil {
		md.Err(err)
		return Err500
	}

	// Convert to API response format
	user.userData = userDataDB.toUserData(eligibleTitles)
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
		Where("users.country = ?", md.Query("country")).
		Where("users.username_aka = ?", md.Query("name_aka")).
		Where("privileges_groups.name = ?", md.Query("privilege_group")).
		In("users.id", pm("ids")...).
		In("users.username_safe", safeUsernameBulk(pm("names"))...).
		In("users.username_aka", pm("names_aka")...).
		In("users.country", pm("countries")...)

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
		var userDB userDataDB

		err := rows.StructScan(&userDB)
		if err != nil {
			md.Err(err)
			continue
		}

		var eligibleTitles []eligibleTitle
		eligibleTitles, err = getEligibleTitles(md, userDB.Privileges)
		if err != nil {
			md.Err(err)
			return Err500
		}

		// Convert to API response format
		u := userDB.toUserData(eligibleTitles)
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
	RankedScore            uint64     `json:"ranked_score"`
	TotalScore             uint64     `json:"total_score"`
	PlayCount              int        `json:"playcount"`
	PlayTime               int        `json:"playtime"`
	ReplaysWatched         int        `json:"replays_watched"`
	TotalHits              int        `json:"total_hits"`
	Level                  float64    `json:"level"`
	Accuracy               float64    `json:"accuracy"`
	PP                     int        `json:"pp"`
	GlobalLeaderboardRank  *int       `json:"global_leaderboard_rank"`
	CountryLeaderboardRank *int       `json:"country_leaderboard_rank"`
	MaxCombo               int        `json:"max_combo"`
	Grades                 userGrades `json:"grades"`
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
	UserTitle     userTitleResponse     `json:"user_title"`
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
	shouldRet, whereClause, userIdParam := whereClauseUser(md, "users")
	if shouldRet != nil {
		return *shouldRet
	}

	r := userFullResponse{}
	var (
		b singleBadge

		can  bool
		show bool
		userDB userDataDB
	)
	// Scan user information into response
	err := md.DB.QueryRow(`
		SELECT
			id, username, register_datetime, privileges, latest_activity,
			username_aka, country, play_style, favourite_mode, custom_badge_icon,
			custom_badge_name, can_custom_badge, show_custom_badge, silence_reason,
			silence_end, notes, ban_datetime, email, clan_id, user_title
		FROM users
		WHERE `+whereClause+` AND `+md.User.OnlyUserPublic(true),
		userIdParam,
	).Scan(
		&userDB.ID, &userDB.Username, &userDB.RegisteredOn, &userDB.Privileges, &userDB.LatestActivity,
		&userDB.UsernameAKA, &userDB.Country, &r.PlayStyle, &r.FavouriteMode, &b.Icon,
		&b.Name, &can, &show, &r.SilenceInfo.Reason,
		&r.SilenceInfo.End, &r.CMNotes, &r.BanDate, &r.Email, &r.Clan.ID, &userDB.UserTitle,
	)
	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "That user could not be found!")
	case err != nil:
		md.Err(err)
		return Err500
	}

	// Scan stats into response for all gamemodes, across vn/rx/ap
	query := `
		SELECT
			user_stats.ranked_score, user_stats.total_score, user_stats.playcount, user_stats.playtime,
			user_stats.replays_watched, user_stats.total_hits,
			user_stats.avg_accuracy, user_stats.pp, user_stats.max_combo,
			user_stats.xh_count, user_stats.x_count, user_stats.sh_count,
			user_stats.s_count, user_stats.a_count, user_stats.b_count,
			user_stats.c_count, user_stats.d_count
		FROM user_stats
		INNER JOIN users ON users.id = user_stats.user_id
		WHERE ` + whereClause + ` AND ` + md.User.OnlyUserPublic(true) + ` AND user_stats.mode = ?
`
	for _, relaxMode := range []int{0, 1, 2} {
		modeOffset := relaxMode * 4
		// Scan vanilla gamemode information into response
		err = md.DB.QueryRow(query, userIdParam, 0+modeOffset).Scan(
			&r.Stats[relaxMode].STD.RankedScore, &r.Stats[relaxMode].STD.TotalScore, &r.Stats[relaxMode].STD.PlayCount, &r.Stats[relaxMode].STD.PlayTime,
			&r.Stats[relaxMode].STD.ReplaysWatched, &r.Stats[relaxMode].STD.TotalHits,
			&r.Stats[relaxMode].STD.Accuracy, &r.Stats[relaxMode].STD.PP, &r.Stats[relaxMode].STD.MaxCombo,
			&r.Stats[relaxMode].STD.Grades.XHCount, &r.Stats[relaxMode].STD.Grades.XCount, &r.Stats[relaxMode].STD.Grades.SHCount,
			&r.Stats[relaxMode].STD.Grades.SCount, &r.Stats[relaxMode].STD.Grades.ACount, &r.Stats[relaxMode].STD.Grades.BCount,
			&r.Stats[relaxMode].STD.Grades.CCount, &r.Stats[relaxMode].STD.Grades.DCount,
		)
		switch {
		case err == sql.ErrNoRows:
			return common.SimpleResponse(404, "That user could not be found!")
		case err != nil:
			md.Err(err)
			return Err500
		}

		if relaxMode == 2 {
			// AP only has osu! standard
			continue
		}

		// Scan taiko gamemode information into response
		err = md.DB.QueryRow(query, userIdParam, 1+modeOffset).Scan(
			&r.Stats[relaxMode].Taiko.RankedScore, &r.Stats[relaxMode].Taiko.TotalScore, &r.Stats[relaxMode].Taiko.PlayCount, &r.Stats[relaxMode].Taiko.PlayTime,
			&r.Stats[relaxMode].Taiko.ReplaysWatched, &r.Stats[relaxMode].Taiko.TotalHits,
			&r.Stats[relaxMode].Taiko.Accuracy, &r.Stats[relaxMode].Taiko.PP, &r.Stats[relaxMode].Taiko.MaxCombo,
			&r.Stats[relaxMode].Taiko.Grades.XHCount, &r.Stats[relaxMode].Taiko.Grades.XCount, &r.Stats[relaxMode].Taiko.Grades.SHCount,
			&r.Stats[relaxMode].Taiko.Grades.SCount, &r.Stats[relaxMode].Taiko.Grades.ACount, &r.Stats[relaxMode].Taiko.Grades.BCount,
			&r.Stats[relaxMode].Taiko.Grades.CCount, &r.Stats[relaxMode].Taiko.Grades.DCount,
		)
		switch {
		case err == sql.ErrNoRows:
			return common.SimpleResponse(404, "That user could not be found!")
		case err != nil:
			md.Err(err)
			return Err500
		}

		// Scan ctb gamemode information into response
		err = md.DB.QueryRow(query, userIdParam, 2+modeOffset).Scan(
			&r.Stats[relaxMode].CTB.RankedScore, &r.Stats[relaxMode].CTB.TotalScore, &r.Stats[relaxMode].CTB.PlayCount, &r.Stats[relaxMode].CTB.PlayTime,
			&r.Stats[relaxMode].CTB.ReplaysWatched, &r.Stats[relaxMode].CTB.TotalHits,
			&r.Stats[relaxMode].CTB.Accuracy, &r.Stats[relaxMode].CTB.PP, &r.Stats[relaxMode].CTB.MaxCombo,
			&r.Stats[relaxMode].CTB.Grades.XHCount, &r.Stats[relaxMode].CTB.Grades.XCount, &r.Stats[relaxMode].CTB.Grades.SHCount,
			&r.Stats[relaxMode].CTB.Grades.SCount, &r.Stats[relaxMode].CTB.Grades.ACount, &r.Stats[relaxMode].CTB.Grades.BCount,
			&r.Stats[relaxMode].CTB.Grades.CCount, &r.Stats[relaxMode].CTB.Grades.DCount,
		)
		switch {
		case err == sql.ErrNoRows:
			return common.SimpleResponse(404, "That user could not be found!")
		case err != nil:
			md.Err(err)
			return Err500
		}

		if relaxMode == 1 {
			// RX does not have mania
			continue
		}

		// Scan mania gamemode information into response
		err = md.DB.QueryRow(query, userIdParam, 3+modeOffset).Scan(
			&r.Stats[relaxMode].Mania.RankedScore, &r.Stats[relaxMode].Mania.TotalScore, &r.Stats[relaxMode].Mania.PlayCount, &r.Stats[relaxMode].Mania.PlayTime,
			&r.Stats[relaxMode].Mania.ReplaysWatched, &r.Stats[relaxMode].Mania.TotalHits,
			&r.Stats[relaxMode].Mania.Accuracy, &r.Stats[relaxMode].Mania.PP, &r.Stats[relaxMode].Mania.MaxCombo,
			&r.Stats[relaxMode].Mania.Grades.XHCount, &r.Stats[relaxMode].Mania.Grades.XCount, &r.Stats[relaxMode].Mania.Grades.SHCount,
			&r.Stats[relaxMode].Mania.Grades.SCount, &r.Stats[relaxMode].Mania.Grades.ACount, &r.Stats[relaxMode].Mania.Grades.BCount,
			&r.Stats[relaxMode].Mania.Grades.CCount, &r.Stats[relaxMode].Mania.Grades.DCount,
		)
		switch {
		case err == sql.ErrNoRows:
			return common.SimpleResponse(404, "That user could not be found!")
		case err != nil:
			md.Err(err)
			return Err500
		}
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

	rows, err = md.DB.Query("SELECT b.id, b.name, b.icon, b.colour FROM user_badges ub "+
		"INNER JOIN badges b ON ub.badge = b.id WHERE user = ?", r.ID)
	if err != nil {
		md.Err(err)
	}

	for rows.Next() {
		var badge singleBadge
		err := rows.Scan(&badge.ID, &badge.Name, &badge.Icon, &badge.Colour)
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
		"INNER JOIN tourmnt_badges tb ON tub.badge = tb.id WHERE user = ?", r.ID)
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

	// Get eligible titles
	eligibleTitles, err := getEligibleTitles(md, userDB.Privileges)
	if err != nil {
		md.Err(err)
		return Err500
	}
	slog.Info("Global std rank before", "rank", r.Stats[0].STD.GlobalLeaderboardRank)
	slog.Info("Followers before", "followers", follower)

	// Convert userDB to userData and set it in the response
	slog.Info("Global std rank after", "rank", r.Stats[0].STD.GlobalLeaderboardRank)
	slog.Info("Followers after", "followers", follower)
	r.userData = userDB.toUserData(eligibleTitles)

	r.Code = 200
	return r
}

// getUserTitleFromID converts a machine-readable title ID to human-readable title
func getUserTitleFromID(titleID string) string {
	titleMap := map[string]string{
		"bot":               "CHAT BOT",
		"product_manager":   "PRODUCT MANAGER",
		"developer":         "PRODUCT DEVELOPER",
		"designer":          "PRODUCT DESIGNER",
		"community_manager": "COMMUNITY MANAGER",
		"community_support": "COMMUNITY SUPPORT",
		"event_manager":     "EVENT MANAGER",
		"nqa":               "NOMINATION QUALITY ASSURANCE",
		"nominator":         "BEATMAP NOMINATOR",
		"scorewatcher":      "SOCIAL MEDIA MANAGER",
		"champion":          "AKATSUKI CHAMPION",
		"premium":           "AKATSUKI+",
		"donor":             "SUPPORTER",
	}

	if title, exists := titleMap[titleID]; exists {
		return title
	}
	return titleID // Return ID if not found (fallback)
}

type userpageResponse struct {
	common.ResponseBase
	Userpage         *string `json:"userpage"`
	UserpageCompiled string  `json:"userpage_compiled"`
}

// UserUserpageGET gets an user's userpage, as in the customisable thing.
func UserUserpageGET(md common.MethodData) common.CodeMessager {
	dontEscape := md.Query("de") == "1"
	shouldRet, whereClause, param := whereClauseUser(md, "users")
	if shouldRet != nil {
		return *shouldRet
	}
	var r userpageResponse
	err := md.DB.QueryRow("SELECT userpage_content FROM users WHERE "+whereClause, param).Scan(&r.Userpage)
	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "No such user!")
	case err != nil:
		md.Err(err)
		return Err500
	}
	r.Code = 200
	if r.Userpage == nil {
		r.Userpage = new(string)
		r.UserpageCompiled = ""
		return r
	}
	if !dontEscape {
		*r.Userpage = html.EscapeString(*r.Userpage)
	}
	r.UserpageCompiled = externals.ConvertBBCodeToHTML(*r.Userpage)
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
	if len(*d.Data) > 65535 {
		return common.SimpleResponse(400, "Userpage content is too long, maximum is 65535 characters")
	}
	cont := common.SanitiseString(*d.Data)
	_, err := md.DB.Exec("UPDATE users SET userpage_content = ? WHERE id = ?", cont, md.ID())
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
