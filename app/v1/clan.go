package v1

import (
	"database/sql"
	"strconv"
	"strings"
	
	"zxq.co/ripple/rippleapi/common"
	"zxq.co/x/rs"
)

type Clan struct {
	ID int				`json:"id"`
	Name string			`json:"name"`
	Tag string			`json:"tag"`
	Description string	`json:"description"`
	Icon string 		`json:"icon"`
	Owner int			`json:"owner"`
}

// clansGET retrieves all the clans on this ripple instance.
func ClansGET(md common.MethodData) common.CodeMessager {
	if md.Query("id") != "" {
		type Res struct {
			common.ResponseBase
			Clan Clan `json:"clan"`
		}
		r := Res{}
		var err error
		r.Clan, err = getClan(common.Int(md.Query("id")), md);
		if err != nil {
			md.Err(err)
			return Err500
		}
		r.ResponseBase.Code = 200
		return r
	}
	type Res struct {
		common.ResponseBase
		Clans []Clan `json:"clans"`
	}
	r := Res{}
	rows, err := md.DB.Query("SELECT id, name, description, tag, icon, owner FROM clans " + common.Paginate(md.Query("p"), md.Query("l"), 50))
	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()
	for rows.Next() {
		var c Clan
		err = rows.Scan(&c.ID, &c.Name, &c.Description, &c.Tag, &c.Icon, &c.Owner)
		if err != nil {
			md.Err(err)
			return Err500
		}
		r.Clans = append(r.Clans, c)
	}
	r.ResponseBase.Code = 200
	return r
}

func ClanLeaderboardGET(md common.MethodData) common.CodeMessager {
	mode := common.Int(md.Query("m"))
	page, err := strconv.Atoi(md.Query("p"))
	if err != nil || page == 0 {
		page = 1
	}
	type clanLbData struct {
		ID int		`json:"id"`
		Name string	`json:"name"`
		ChosenMode modeData `json:"chosen_mode"`
	}
	type clanLeaderboard struct {
		Page int			`json:"page"`
		Clans []clanLbData	`json:"clans"`
	}
	relax := md.Query("rx") == "1"
	tableName := "users"
	if relax {
		tableName = "rx"
	}
	cl := clanLeaderboard{Page: page}
	q := strings.Replace("SELECT SUM(pp_DBMODE)/(COUNT(clan_id)+1) AS pp, SUM(ranked_score_DBMODE), SUM(total_score_DBMODE), SUM(playcount_DBMODE), AVG(avg_accuracy_DBMODE), clans.name, clans.id FROM " + tableName + "_stats LEFT JOIN users ON users.id = " + tableName + "_stats.id INNER JOIN clans ON clans.id=users.clan_id WHERE clan_id <> 0 AND (users.privileges&3)>=3 GROUP BY clan_id ORDER BY pp DESC LIMIT ?,50", "DBMODE", dbmode[mode], -1)
	rows, err := md.DB.Query(q, (page-1)*50)
	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()
	for i := 1; rows.Next(); i++ {
		clan := clanLbData{}
		var pp float64
		rows.Scan(&pp, &clan.ChosenMode.RankedScore, &clan.ChosenMode.TotalScore, &clan.ChosenMode.PlayCount, &clan.ChosenMode.Accuracy, &clan.Name, &clan.ID)
		if err != nil {
			md.Err(err)
			return Err500
		}
		clan.ChosenMode.PP = int(pp)
		rank := i + (page - 1) * 50
		clan.ChosenMode.GlobalLeaderboardRank = &rank
		cl.Clans = append(cl.Clans, clan)
	}
	type Res struct {
		common.ResponseBase
		clanLeaderboard
	}
	r := Res{clanLeaderboard: cl}
	r.ResponseBase.Code = 200
	return r
}

var dbmode = [...]string{"std", "taiko","ctb","mania"}

func ClanStatsGET(md common.MethodData) common.CodeMessager {
	if md.Query("id") == "" {
		return ErrMissingField("id")
	}
	id, err := strconv.Atoi(md.Query("id"))
	if err != nil {
			return common.SimpleResponse(400, "please pass a valid ID")
	}
	mode := common.Int(md.Query("m"))
	
	type clanModeStats struct {
		Clan
		ChosenMode modeData `json:"chosen_mode"`
	}

	relax := md.Query("rx") == "1"
	tableName := "users"
	if relax {
		tableName = "rx"
	}
	type Res struct {
		common.ResponseBase
		Clan clanModeStats `json:"clan"`
	}
	cms := clanModeStats{}
	cms.Clan, err = getClan(id, md)
	if err != nil {
		return Res{Clan:cms}
	}
	q := strings.Replace("SELECT SUM(pp_DBMODE)/(COUNT(clan_id)+1) AS pp, SUM(ranked_score_DBMODE), SUM(total_score_DBMODE), SUM(playcount_DBMODE), SUM(replays_watched_DBMODE), AVG(avg_accuracy_DBMODE), SUM(total_hits_DBMODE) FROM " + tableName + "_stats LEFT JOIN users ON users.id = " + tableName + "_stats.id WHERE users.clan_id = ? AND (users.privileges & 3) >= 3 LIMIT 1", "DBMODE", dbmode[mode], -1)
	var pp float64
	err = md.DB.QueryRow(q, id).Scan(&pp, &cms.ChosenMode.RankedScore, &cms.ChosenMode.TotalScore, &cms.ChosenMode.PlayCount, &cms.ChosenMode.ReplaysWatched, &cms.ChosenMode.Accuracy, &cms.ChosenMode.TotalHits)
	if err != nil {
		md.Err(err)
		return Err500
	}
	
	cms.ChosenMode.PP = int(pp)
	var rank int
	err = md.DB.QueryRow("SELECT COUNT(pp) FROM (SELECT SUM(pp_" + dbmode[mode] + ")/(COUNT(clan_id)+1) AS pp FROM " + tableName + "_stats LEFT JOIN users ON users.id = " + tableName + "_stats.id WHERE clan_id <> 0 AND (users.privileges&3)>=3 GROUP BY clan_id) x WHERE x.pp >= ?", cms.ChosenMode.PP).Scan(&rank)
	if err != nil {
		md.Err(err)
		return Err500
	}
	cms.ChosenMode.GlobalLeaderboardRank = &rank
	r := Res{Clan: cms}
	r.ResponseBase.Code = 200
	return r
}

func ResolveInviteGET(md common.MethodData) common.CodeMessager {
	s := md.Query("invite")
	if s == "" {
		return ErrMissingField("invite")
	}
	type Res struct {
		common.ResponseBase
		Clan Clan `json:"clan"`
	}
	
	clan := Clan{}
	err := md.DB.QueryRow("SELECT id, name, description, tag, icon, owner FROM clans WHERE invite = ? LIMIT 1", s).Scan(&clan.ID, &clan.Name, &clan.Description, &clan.Tag, &clan.Icon, &clan.Owner)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.SimpleResponse(404, "No clan with given invite found.")
		} else {
			md.Err(err)
			return Err500	
		}
	}
	r := Res{Clan: clan}
	r.ResponseBase.Code = 200
	return r
}

func ClanJoinPOST(md common.MethodData) common.CodeMessager {
	if md.ID() == 0 {
		return common.SimpleResponse(401, "not authorised")
	}
	
	var cID int
	err := md.DB.QueryRow("SELECT clan_id FROM users WHERE id = ?", md.ID()).Scan(&cID)
	if err != nil {
		md.Err(err)
		return Err500
	}
	if cID != 0 {
		return common.SimpleResponse(403, "already joined a clan")
	}
	
	var u struct {
		ID int `json:"id,omitempty"`
		Invite string `json:"invite,omitempty"`
	}
	
	md.Unmarshal(&u)
	if u.ID == 0 && u.Invite == "" {
		return common.SimpleResponse(400, "id or invite required")
	}
	
	r := struct {
		common.ResponseBase
		Clan Clan `json:"clan"`
	}{}
	var hInvite bool
	hinvite:
	if u.ID > 0 {
		var status int
		err = md.DB.QueryRow("SELECT status FROM clans WHERE id = ?", u.ID).Scan(&status)
		if status == 0 || (status == 2 && !hInvite) {
			return common.SimpleResponse(200, "closed")
		}
		c, err := getClan(u.ID, md)
		if err != nil {
			if err == sql.ErrNoRows {
				return common.SimpleResponse(404, "clan not found")
			}
			md.Err(err)
			return Err500
		}
		_, err = md.DB.Exec("UPDATE users SET clan_id = ? WHERE id = ?", u.ID, md.ID())
		r.Clan = c
		r.Code = 200
	} else if u.Invite != "" {
		if len(u.Invite) != 8 {
			return common.SimpleResponse(400, "invalid invite parameter")
		}
		err := md.DB.QueryRow("SELECT id FROM clans WHERE invite = ?", u.Invite).Scan(&u.ID)
		if err != nil {
			if err == sql.ErrNoRows {
				return common.SimpleResponse(404, "clan not found")
			}
			md.Err(err)
			return Err500
		}
		u.Invite = ""
		hInvite = true
		goto hinvite
	} else if u.ID <= 0 {
		return common.SimpleResponse(400, "invalid id parameter")
	}
	
	return r
}

func ClanGenerateInvitePOST(md common.MethodData) common.CodeMessager {
	if md.ID() == 0 {
		return common.SimpleResponse(401, "not authorised")
	}
	
	var id int
	err := md.DB.QueryRow("SELECT id FROM clans WHERE owner = ?", md.ID()).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.SimpleResponse(401, "not authorised")
		}
		md.Err(err)
		return Err500
	}
	
	invite := rs.String(8)
	err = md.DB.Exec("UPDATE clans SET invite = ? WHERE id = ?", invite, id)
	if err != nil {
		md.Err(err)
		return Err500
	}
	
	r := struct {
		common.ResponseBase
		Invite string `json:"invite"`
	}{Code: 200, Invite: invite}
	
	return r
}

// ClanMembersGET retrieves the people who are in a certain clan.
func ClanMembersGET(md common.MethodData) common.CodeMessager {
	i := common.Int(md.Query("id"))
	if i == 0 {
		return ErrMissingField("id")
	}
	type clanMembersData struct {
		Clan
		Members []userData `json:"members"`
	}
	cmd := clanMembersData{}
	var err error
	cmd.Clan, err = getClan(i, md)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.SimpleResponse(404, "clan not found")
		}
		md.Err(err)
		return Err500
	}

	rows, err := md.DB.Query(userFields + " WHERE users.privileges & 3 AND clan_id = ?", i);
	if err != nil {
		if err == sql.ErrNoRows {
			return struct {
				common.ResponseBase
				Clan clanMembersData `json:"clan"`
			}{Clan: cmd}
		}
		md.Err(err)
		return Err500
	}
	defer rows.Close()
	for rows.Next() {
		a := userData{}

		err = rows.Scan(&a.ID, &a.Username, &a.RegisteredOn, &a.Privileges, &a.LatestActivity, &a.UsernameAKA, &a.Country)
		if err != nil {
			md.Err(err)
			return Err500
		}
		cmd.Members = append(cmd.Members, a)
	}
	res := struct {
		common.ResponseBase
		Clan clanMembersData `json:"clan"`
	}{Clan: cmd}
	res.ResponseBase.Code = 200
	return res
}

func getClan(id int, md common.MethodData) (Clan, error) {
	c := Clan{}
	if id == 0 {
		return c, nil // lol?
	}
	err := md.DB.QueryRow("SELECT id, name, description, tag, icon, owner FROM clans WHERE id = ? LIMIT 1", id).Scan(&c.ID, &c.Name, &c.Description, &c.Tag, &c.Icon, &c.Owner)
	if err != nil {
		return c, err
	}
	return c, nil
}

func getUserData(id int, md common.MethodData) (userData, error) {
	u := userData{}
	if id == 0 {
		return u, nil
	}
	err := md.DB.QueryRow("SELECT users.id, users.username, register_datetime, privileges, latest_activity, username_aka, country FROM users LEFT JOIN users_stats ON users.id = users_stats.id WHERE users.id = ? LIMIT 1", id).Scan(&u.ID, &u.Username, &u.RegisteredOn, &u.Privileges, &u.LatestActivity, &u.UsernameAKA, &u.Country)
	if err != nil {
		return u, err
	}
	return u, nil
}
