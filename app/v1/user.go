package v1

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"gopkg.in/thehowl/go-osuapi.v1"

	"github.com/osuAkatsuki/akatsuki-api/common"
	"zxq.co/ripple/ocl"
)

// userDataDB is the structure used to scan users from the database
type userDataDB struct {
	ID                 int                  `db:"id" json:"id"`
	Username           string               `db:"username" json:"username"`
	UsernameAKA        string               `db:"username_aka" json:"username_aka"`
	RegisteredOn       common.UnixTimestamp `db:"register_datetime" json:"registered_on"`
	Privileges         uint64               `db:"privileges" json:"privileges"`
	LatestActivity     common.UnixTimestamp `db:"latest_activity" json:"latest_activity"`
	Country            string               `db:"country" json:"country"`
	UserTitle          string               `db:"user_title" json:"user_title"`
}

// userData is the user data meant for API responses
type userData struct {
	ID             int                  `json:"id"`
	Username       string               `json:"username"`
	UsernameAKA    string               `json:"username_aka"`
	RegisteredOn   common.UnixTimestamp `json:"registered_on"`
	Privileges     uint64               `json:"privileges"`
	LatestActivity common.UnixTimestamp `json:"latest_activity"`
	Country        string               `json:"country"`
	UserTitle      string               `json:"user_title"`
}

// toUserData converts userDataDB to userData with eligibility check
func (u *userDataDB) toUserData(eligibleTitles []eligibleTitle) userData {
	userTitle := u.UserTitle
	if !titleIsEligible(u.ID, u.Privileges, eligibleTitles) {
		userTitle = ""
	}

	return userData{
		ID:             u.ID,
		Username:       u.Username,
		UsernameAKA:    u.UsernameAKA,
		RegisteredOn:   u.RegisteredOn,
		Privileges:     u.Privileges,
		LatestActivity: u.LatestActivity,
		Country:        u.Country,
		UserTitle:      userTitle,
	}
}

const userFields = `SELECT users.id, users.username, users.register_datetime, users.privileges,
		users.latest_activity, users.username_aka, users.country, users.user_title
		FROM users `

func userByID(md common.MethodData, id int) common.CodeMessager {
	return userPutsSingle(md, md.DB.QueryRowx(userFields+"WHERE users.id = ? AND "+md.User.OnlyUserPublic(true)+" LIMIT 1", id))
}

func userByName(md common.MethodData, name string) common.CodeMessager {
	var whereClause string
	var param string

	if strings.Contains(name, " ") {
		whereClause = "users.username = ?"
		param = name
	} else {
		whereClause = "users.username_safe = ?"
		param = common.SafeUsername(name)
	}

	query := userFields + `WHERE ` + whereClause + ` AND ` + md.User.OnlyUserPublic(true) + `
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
		Allowed: []string{"id", "username", "privileges", "donor_expire", "latest_activity", "silence_end"},
		Default: "id ASC",
		Table:   "users",
	}) +
		" " + common.Paginate(md.Query("p"), md.Query("l"), 100)

	// query execution
	rows, err := md.DB.Queryx(query, wh.Params...)
	if err != nil { md.Err(err); return Err500 }

	var r userPutsMultiUserData
	for rows.Next() {
		var userDB userDataDB
		if err := rows.StructScan(&userDB); err != nil { md.Err(err); continue }
		eligibleTitles, err := getEligibleTitles(md, userDB.ID, userDB.Privileges)
		if err != nil { md.Err(err); return Err500 }
		r.Users = append(r.Users, userDB.toUserData(eligibleTitles))
	}
	r.Code = 200
	return r
}

// UserSelfGET is a shortcut for /users/id/self. (/users/self)
