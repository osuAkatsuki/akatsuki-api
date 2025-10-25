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
	} else if len(eligibleTitles) > 0 {
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
	eligibleTitles, err = getEligibleTitles(md, userDataDB.ID, userDataDB.Privileges)
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
		Where("users.username_safe = ?", common.SafeUsername(md.Query("nname")).
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
		eligibleTitles, err = getEligibleTitles(md, userDB.ID, userDB.Privileges)
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
	PPTotal                float64    `json:"pp_total"`
	PPStddev               float64    `json:"pp_stddev"`
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
	eligibleTitles, err := getEligibleTitles(md, userDB.ID, userDB.Privileges)
	if err != nil {
		md.Err(err)
		return Err500
	}
	r.userData = userDB.toUserData(eligibleTitles)
	// Scan stats into response for all gamemodes, across vn/rx/ap
	query := `
		SELECT
			user_stats.ranked_score, user_stats.total_score, user_stats.playcount, user_stats.playtime,
			user_stats.replays_watched, user_stats.total_hits,
			user_stats.avg_accuracy, user_stats.pp, user_stats.pp_total, user_stats.pp_stddev, user_stats.max_combo,
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
			&r.Stats[relaxMode].STD.Accuracy, &r.Stats[relaxMode].STD.PP, &r.Stats[relaxMode].STD.PPTotal, &r.Stats[relaxMode].STD.PPStddev, &r.Stats[relaxMode].STD.MaxCombo,
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
			&r.Stats[relaxMode].Taiko.RankedScore, &r.Stats[relaxMode].Taiko.TotalScore, &r.Stats
