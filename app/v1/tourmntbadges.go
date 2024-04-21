package v1

import (
	"database/sql"

	"github.com/osuAkatsuki/akatsuki-api/common"
)

type TsingleBadge struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

type TmultiBadgeData struct {
	common.ResponseBase
	Badges []TsingleBadge `json:"tbadges"`
}

// BadgesGET retrieves all the badges on this ripple instance.
func TBadgesGET(md common.MethodData) common.CodeMessager {
	var (
		r    TmultiBadgeData
		rows *sql.Rows
		err  error
	)
	if md.Query("id") != "" {
		rows, err = md.DB.Query("SELECT id, name, icon FROM tourmnt_badges WHERE id = ? LIMIT 1", md.Query("id"))
	} else {
		rows, err = md.DB.Query("SELECT id, name, icon FROM tourmnt_badges " + common.Paginate(md.Query("p"), md.Query("l"), 50))
	}
	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()
	for rows.Next() {
		nb := TsingleBadge{}
		err = rows.Scan(&nb.ID, &nb.Name, &nb.Icon)
		if err != nil {
			md.Err(err)
		}
		r.Badges = append(r.Badges, nb)
	}
	if err := rows.Err(); err != nil {
		md.Err(err)
	}
	r.ResponseBase.Code = 200
	return r
}

type TbadgeMembersData struct {
	common.ResponseBase
	Members []userData `json:"members"`
}

// BadgeMembersGET retrieves the people who have a certain badge.
func TBadgeMembersGET(md common.MethodData) common.CodeMessager {
	i := common.Int(md.Query("id"))
	if i == 0 {
		return ErrMissingField("id")
	}

	var members TbadgeMembersData

	err := md.DB.Select(&members.Members, `SELECT users.id, users.username, users.register_datetime,
	users.privileges, users.latest_activity, users.username_aka, users.country
FROM user_tourmnt_badges ub
INNER JOIN users ON users.id = ub.user
WHERE badge = ?
ORDER BY id ASC `+common.Paginate(md.Query("p"), md.Query("l"), 50), i)

	if err != nil {
		md.Err(err)
		return Err500
	}

	members.Code = 200
	return members
}
