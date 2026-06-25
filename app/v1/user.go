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
	redis "gopkg.in/redis.v5"

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

type silenceInfo struct {
	Reason string               `json:"reason"`
	End    common.UnixTimestamp `json:"end"`
}

const maxUserFullBulkIDs = 50

var userFullStatModes = []int{0, 1, 2, 3, 4, 5, 6, 8}

type userFullData struct {
	userData
	CountryName   string                `json:"country_name"`
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

type userFullResponse struct {
	common.ResponseBase
	userFullData
}

type userFullMultiResponse struct {
	common.ResponseBase
	Users []userFullData `json:"users"`
}

type userFullDB struct {
	userDataDB
	PlayStyle       int                   `db:"play_style"`
	FavouriteMode   int                   `db:"favourite_mode"`
	CustomBadgeIcon string                `db:"custom_badge_icon"`
	CustomBadgeName string                `db:"custom_badge_name"`
	CanCustomBadge  bool                  `db:"can_custom_badge"`
	ShowCustomBadge bool                  `db:"show_custom_badge"`
	SilenceReason   string                `db:"silence_reason"`
	SilenceEnd      common.UnixTimestamp  `db:"silence_end"`
	CMNotes         *string               `db:"notes"`
	BanDate         *common.UnixTimestamp `db:"ban_datetime"`
	Email           string                `db:"email"`
	ClanID          int                   `db:"clan_id"`
}

type userFullRankCommand struct {
	cmd  *redis.IntCmd
	dest **int
}

const userFullFields = `
SELECT
	users.id, users.username, users.register_datetime, users.privileges,
	users.latest_activity, users.username_aka, users.country, users.user_title,
	users.play_style, users.favourite_mode, users.custom_badge_icon,
	users.custom_badge_name, users.can_custom_badge, users.show_custom_badge,
	users.silence_reason, users.silence_end, users.notes, users.ban_datetime,
	users.email, users.clan_id
FROM users
`

// UserFullGET gets all of an user's information, with one exception: their userpage.
func UserFullGET(md common.MethodData) common.CodeMessager {
	if len(md.Ctx.QueryArgs().PeekMulti("ids")) > 0 {
		return userFullPutsMulti(md)
	}

	shouldRet, whereClause, userIdParam := whereClauseUser(md, "users")
	if shouldRet != nil {
		return *shouldRet
	}

	query := userFullFields + `
WHERE ` + whereClause + ` AND ` + md.User.OnlyUserPublic(true) + `
LIMIT 1`
	users, errResp := getUserFullData(md, query, userIdParam)
	if errResp != nil {
		return errResp
	}
	if len(users) == 0 {
		return common.SimpleResponse(404, "That user could not be found!")
	}

	return userFullResponse{
		ResponseBase: common.ResponseBase{Code: 200},
		userFullData: users[0],
	}
}

func userFullPutsMulti(md common.MethodData) common.CodeMessager {
	if md.Query("id") != "" || md.Query("name") != "" {
		return common.SimpleResponse(400, "please pass either id/name or ids, not both")
	}

	ids, errResp := parseUserFullIDs(md.Ctx.QueryArgs().PeekMulti("ids"))
	if errResp != nil {
		return errResp
	}

	query, params, err := sqlx.In(
		userFullFields+`WHERE users.id IN (?) AND `+md.User.OnlyUserPublic(true),
		ids,
	)
	if err != nil {
		md.Err(err)
		return Err500
	}

	users, errResp := getUserFullData(md, query, params...)
	if errResp != nil {
		return errResp
	}

	usersByID := make(map[int]userFullData, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}

	orderedUsers := make([]userFullData, 0, len(users))
	for _, id := range ids {
		if user, ok := usersByID[id]; ok {
			orderedUsers = append(orderedUsers, user)
		}
	}

	return userFullMultiResponse{
		ResponseBase: common.ResponseBase{Code: 200},
		Users:        orderedUsers,
	}
}

func parseUserFullIDs(rawIDs [][]byte) ([]int, common.CodeMessager) {
	if len(rawIDs) > maxUserFullBulkIDs {
		return nil, common.SimpleResponse(400, "a maximum of 50 user IDs can be requested")
	}

	ids := make([]int, 0, len(rawIDs))
	seen := make(map[int]bool, len(rawIDs))
	for _, rawID := range rawIDs {
		id, err := strconv.Atoi(string(rawID))
		if err != nil || id <= 0 {
			return nil, common.SimpleResponse(400, "please pass valid user IDs")
		}

		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}

	return ids, nil
}

func getUserFullData(md common.MethodData, query string, params ...interface{}) ([]userFullData, common.CodeMessager) {
	rows, err := md.DB.Queryx(query, params...)
	if err != nil {
		md.Err(err)
		return nil, Err500
	}
	defer rows.Close()

	baseUsers := make([]userFullDB, 0)
	for rows.Next() {
		var user userFullDB
		if err := rows.StructScan(&user); err != nil {
			md.Err(err)
			return nil, Err500
		}
		baseUsers = append(baseUsers, user)
	}
	if err := rows.Err(); err != nil {
		md.Err(err)
		return nil, Err500
	}

	if len(baseUsers) == 0 {
		return []userFullData{}, nil
	}

	return buildUserFullData(md, baseUsers)
}

func buildUserFullData(md common.MethodData, baseUsers []userFullDB) ([]userFullData, common.CodeMessager) {
	ids := make([]int, 0, len(baseUsers))
	clanIDs := make([]int, 0, len(baseUsers))
	for _, user := range baseUsers {
		ids = append(ids, user.ID)
		if user.ClanID != 0 {
			clanIDs = append(clanIDs, user.ClanID)
		}
	}

	badgesByUser, badgeIDsByUser, errResp := getUserFullBadges(md, ids)
	if errResp != nil {
		return nil, errResp
	}
	tbadgesByUser, errResp := getUserFullTBadges(md, ids)
	if errResp != nil {
		return nil, errResp
	}
	followersByUser, errResp := getUserFullFollowers(md, ids)
	if errResp != nil {
		return nil, errResp
	}
	clansByID, errResp := getUserFullClans(md, clanIDs)
	if errResp != nil {
		return nil, errResp
	}

	users := make([]userFullData, len(baseUsers))
	usersByID := make(map[int]*userFullData, len(baseUsers))
	for i, baseUser := range baseUsers {
		eligibleTitles := getEligibleTitlesFromBadgeIDs(baseUser.ID, baseUser.Privileges, badgeIDsByUser)
		user := baseUser.toUserFullData(eligibleTitles)
		user.Badges = badgesByUser[baseUser.ID]
		user.TBadges = tbadgesByUser[baseUser.ID]
		user.Followers = followersByUser[baseUser.ID]
		user.Clan = clansByID[baseUser.ClanID]

		if md.User.TokenPrivileges&common.PrivilegeManageUser == 0 {
			user.CMNotes = nil
			user.BanDate = nil
			user.Email = ""
		}

		users[i] = user
		usersByID[user.ID] = &users[i]
	}

	if errResp := loadUserFullStats(md, ids, usersByID); errResp != nil {
		return nil, errResp
	}
	if errResp := loadUserFullRanks(md, users); errResp != nil {
		return nil, errResp
	}

	return users, nil
}

func (user *userFullDB) toUserFullData(eligibleTitles []eligibleTitle) userFullData {
	userData := user.userDataDB.toUserData(eligibleTitles)
	r := userFullData{
		userData:      userData,
		CountryName:   countryName(user.Country),
		PlayStyle:     user.PlayStyle,
		FavouriteMode: user.FavouriteMode,
		SilenceInfo: silenceInfo{
			Reason: user.SilenceReason,
			End:    user.SilenceEnd,
		},
		CMNotes: user.CMNotes,
		BanDate: user.BanDate,
		Email:   user.Email,
	}

	canCustomBadge := user.CanCustomBadge &&
		user.ShowCustomBadge &&
		common.UserPrivileges(user.Privileges)&common.UserPrivilegeDonor > 0
	if canCustomBadge && (user.CustomBadgeName != "" || user.CustomBadgeIcon != "") {
		r.CustomBadge = &singleBadge{
			Name: user.CustomBadgeName,
			Icon: user.CustomBadgeIcon,
		}
	}

	return r
}

func getUserFullBadges(md common.MethodData, userIDs []int) (map[int][]singleBadge, map[int]map[int]bool, common.CodeMessager) {
	query, params, err := sqlx.In(
		`SELECT ub.user, b.id, b.name, b.icon, b.colour
		FROM user_badges ub
		INNER JOIN badges b ON ub.badge = b.id
		WHERE ub.user IN (?)`,
		userIDs,
	)
	if err != nil {
		md.Err(err)
		return nil, nil, Err500
	}

	rows, err := md.DB.Query(query, params...)
	if err != nil {
		md.Err(err)
		return nil, nil, Err500
	}
	defer rows.Close()

	badgesByUser := make(map[int][]singleBadge)
	badgeIDsByUser := make(map[int]map[int]bool)
	for rows.Next() {
		var userID int
		var badge singleBadge
		if err := rows.Scan(&userID, &badge.ID, &badge.Name, &badge.Icon, &badge.Colour); err != nil {
			md.Err(err)
			return nil, nil, Err500
		}
		badgesByUser[userID] = append(badgesByUser[userID], badge)
		if badgeIDsByUser[userID] == nil {
			badgeIDsByUser[userID] = make(map[int]bool)
		}
		badgeIDsByUser[userID][badge.ID] = true
	}
	if err := rows.Err(); err != nil {
		md.Err(err)
		return nil, nil, Err500
	}

	return badgesByUser, badgeIDsByUser, nil
}

func getUserFullTBadges(md common.MethodData, userIDs []int) (map[int][]TsingleBadge, common.CodeMessager) {
	query, params, err := sqlx.In(
		`SELECT tub.user, tb.id, tb.name, tb.icon
		FROM user_tourmnt_badges tub
		INNER JOIN tourmnt_badges tb ON tub.badge = tb.id
		WHERE tub.user IN (?)`,
		userIDs,
	)
	if err != nil {
		md.Err(err)
		return nil, Err500
	}

	rows, err := md.DB.Query(query, params...)
	if err != nil {
		md.Err(err)
		return nil, Err500
	}
	defer rows.Close()

	tbadgesByUser := make(map[int][]TsingleBadge)
	for rows.Next() {
		var userID int
		var badge TsingleBadge
		if err := rows.Scan(&userID, &badge.ID, &badge.Name, &badge.Icon); err != nil {
			md.Err(err)
			return nil, Err500
		}
		tbadgesByUser[userID] = append(tbadgesByUser[userID], badge)
	}
	if err := rows.Err(); err != nil {
		md.Err(err)
		return nil, Err500
	}

	return tbadgesByUser, nil
}

func getUserFullFollowers(md common.MethodData, userIDs []int) (map[int]int, common.CodeMessager) {
	query, params, err := sqlx.In(
		`SELECT user2, COUNT(id)
		FROM users_relationships
		WHERE user2 IN (?)
		GROUP BY user2`,
		userIDs,
	)
	if err != nil {
		md.Err(err)
		return nil, Err500
	}

	rows, err := md.DB.Query(query, params...)
	if err != nil {
		md.Err(err)
		return nil, Err500
	}
	defer rows.Close()

	followersByUser := make(map[int]int)
	for rows.Next() {
		var userID int
		var followers int
		if err := rows.Scan(&userID, &followers); err != nil {
			md.Err(err)
			return nil, Err500
		}
		followersByUser[userID] = followers
	}
	if err := rows.Err(); err != nil {
		md.Err(err)
		return nil, Err500
	}

	return followersByUser, nil
}

func getUserFullClans(md common.MethodData, clanIDs []int) (map[int]Clan, common.CodeMessager) {
	clanIDs = uniqueInts(clanIDs)
	if len(clanIDs) == 0 {
		return map[int]Clan{}, nil
	}

	query, params, err := sqlx.In(
		`SELECT id, name, description, tag, icon, owner, status
		FROM clans
		WHERE id IN (?)`,
		clanIDs,
	)
	if err != nil {
		md.Err(err)
		return nil, Err500
	}

	rows, err := md.DB.Query(query, params...)
	if err != nil {
		md.Err(err)
		return nil, Err500
	}
	defer rows.Close()

	clansByID := make(map[int]Clan)
	for rows.Next() {
		var clan Clan
		if err := rows.Scan(&clan.ID, &clan.Name, &clan.Description, &clan.Tag, &clan.Icon, &clan.Owner, &clan.Status); err != nil {
			md.Err(err)
			return nil, Err500
		}
		clansByID[clan.ID] = clan
	}
	if err := rows.Err(); err != nil {
		md.Err(err)
		return nil, Err500
	}

	return clansByID, nil
}

func loadUserFullStats(md common.MethodData, userIDs []int, usersByID map[int]*userFullData) common.CodeMessager {
	query, params, err := sqlx.In(
		`SELECT
			user_id, mode, ranked_score, total_score, playcount, playtime,
			replays_watched, total_hits, avg_accuracy, pp, max_combo,
			xh_count, x_count, sh_count, s_count, a_count, b_count, c_count, d_count
		FROM user_stats
		WHERE user_id IN (?) AND mode IN (?)`,
		userIDs,
		userFullStatModes,
	)
	if err != nil {
		md.Err(err)
		return Err500
	}

	rows, err := md.DB.Query(query, params...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()

	seenModesByUser := make(map[int]map[int]bool)
	for rows.Next() {
		var userID int
		var mode int
		var stats modeData
		err := rows.Scan(
			&userID, &mode, &stats.RankedScore, &stats.TotalScore, &stats.PlayCount, &stats.PlayTime,
			&stats.ReplaysWatched, &stats.TotalHits, &stats.Accuracy, &stats.PP, &stats.MaxCombo,
			&stats.Grades.XHCount, &stats.Grades.XCount, &stats.Grades.SHCount,
			&stats.Grades.SCount, &stats.Grades.ACount, &stats.Grades.BCount,
			&stats.Grades.CCount, &stats.Grades.DCount,
		)
		if err != nil {
			md.Err(err)
			return Err500
		}

		user := usersByID[userID]
		if user == nil {
			continue
		}
		target := userFullModeData(&user.Stats, mode)
		if target == nil {
			continue
		}
		*target = stats
		if seenModesByUser[userID] == nil {
			seenModesByUser[userID] = make(map[int]bool)
		}
		seenModesByUser[userID][mode] = true
	}
	if err := rows.Err(); err != nil {
		md.Err(err)
		return Err500
	}

	for userID := range usersByID {
		for _, mode := range userFullStatModes {
			if !seenModesByUser[userID][mode] {
				return common.SimpleResponse(404, "That user could not be found!")
			}
		}
	}

	return nil
}

func userFullModeData(stats *[3]userStats, mode int) *modeData {
	switch mode {
	case 0:
		return &stats[0].STD
	case 1:
		return &stats[0].Taiko
	case 2:
		return &stats[0].CTB
	case 3:
		return &stats[0].Mania
	case 4:
		return &stats[1].STD
	case 5:
		return &stats[1].Taiko
	case 6:
		return &stats[1].CTB
	case 8:
		return &stats[2].STD
	}
	return nil
}

func loadUserFullRanks(md common.MethodData, users []userFullData) common.CodeMessager {
	pipe := md.R.Pipeline()
	defer pipe.Close()

	commands := make([]userFullRankCommand, 0, len(users)*18)
	for i := range users {
		queueUserFullRanks(pipe, &commands, &users[i])
	}
	if len(commands) == 0 {
		return nil
	}

	_, err := pipe.Exec()
	if err != nil && err != redis.Nil {
		md.Err(err)
		return Err500
	}

	for _, command := range commands {
		err := command.cmd.Err()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			md.Err(err)
			return Err500
		}
		rank := int(command.cmd.Val()) + 1
		*command.dest = &rank
	}

	return nil
}

func queueUserFullRanks(pipe *redis.Pipeline, commands *[]userFullRankCommand, user *userFullData) {
	for modeID, stats := range [...]*modeData{&user.Stats[0].STD, &user.Stats[0].Taiko, &user.Stats[0].CTB, &user.Stats[0].Mania} {
		stats.Level = ocl.GetLevelPrecise(int64(stats.TotalScore))
		mode := modesToReadable[modeID]
		queueUserFullRank(pipe, commands, "ripple:leaderboard:"+mode, user.ID, &stats.GlobalLeaderboardRank)
		queueUserFullRank(pipe, commands, "ripple:leaderboard:"+mode+":"+strings.ToLower(user.Country), user.ID, &stats.CountryLeaderboardRank)
	}
	for modeID, stats := range [...]*modeData{&user.Stats[1].STD, &user.Stats[1].Taiko, &user.Stats[1].CTB, &user.Stats[1].Mania} {
		stats.Level = ocl.GetLevelPrecise(int64(stats.TotalScore))
		mode := modesToReadable[modeID]
		queueUserFullRank(pipe, commands, "ripple:relaxboard:"+mode, user.ID, &stats.GlobalLeaderboardRank)
		queueUserFullRank(pipe, commands, "ripple:relaxboard:"+mode+":"+strings.ToLower(user.Country), user.ID, &stats.CountryLeaderboardRank)
	}
	for modeID, stats := range [...]*modeData{&user.Stats[2].STD} {
		stats.Level = ocl.GetLevelPrecise(int64(stats.TotalScore))
		mode := modesToReadable[modeID]
		queueUserFullRank(pipe, commands, "ripple:autoboard:"+mode, user.ID, &stats.GlobalLeaderboardRank)
		queueUserFullRank(pipe, commands, "ripple:autoboard:"+mode+":"+strings.ToLower(user.Country), user.ID, &stats.CountryLeaderboardRank)
	}
}

func queueUserFullRank(pipe *redis.Pipeline, commands *[]userFullRankCommand, key string, userID int, dest **int) {
	*commands = append(*commands, userFullRankCommand{
		cmd:  pipe.ZRevRank(key, strconv.Itoa(userID)),
		dest: dest,
	})
}

func getEligibleTitlesFromBadgeIDs(userID int, privileges uint64, badgeIDsByUser map[int]map[int]bool) []eligibleTitle {
	userBadgeIDs := badgeIDsByUser[userID]
	return getEligibleTitlesFromFlags(
		privileges,
		userBadgeIDs[34],
		userBadgeIDs[101],
		userBadgeIDs[86],
		userBadgeIDs[67],
	)
}

func uniqueInts(values []int) []int {
	seen := make(map[int]bool, len(values))
	uniqueValues := make([]int, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			uniqueValues = append(uniqueValues, value)
		}
	}
	return uniqueValues
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
