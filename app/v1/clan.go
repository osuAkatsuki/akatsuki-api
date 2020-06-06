package v1

import (
	"database/sql"
	"regexp"
	"strconv"
	"strings"

	"zxq.co/ripple/rippleapi/common"
	"zxq.co/x/rs"
)

type Clan struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Tag         string `json:"tag"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Owner       int    `json:"owner"`
	Status		int	   `json:"status"`
}

const clanMemberLimit = 20

// clansGET retrieves all the clans on this ripple instance.
func ClansGET(md common.MethodData) common.CodeMessager {
	if md.Query("id") != "" {
		type Res struct {
			common.ResponseBase
			Clan Clan `json:"clan"`
		}
		r := Res{}
		var err error
		r.Clan, err = getClan(common.Int(md.Query("id")), md)
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
	rows, err := md.DB.Query("SELECT id, name, description, tag, icon, owner, status FROM clans " + common.Paginate(md.Query("p"), md.Query("l"), 50))
	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()
	for rows.Next() {
		var c Clan
		err = rows.Scan(&c.ID, &c.Name, &c.Description, &c.Tag, &c.Icon, &c.Owner, &c.Status)
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
		ID         int      `json:"id"`
		Name       string   `json:"name"`
		ChosenMode modeData `json:"chosen_mode"`
	}
	type clanLeaderboard struct {
		Page  int          `json:"page"`
		Clans []clanLbData `json:"clans"`
	}
	relax := md.Query("rx") == "1"
	tableName := "users"
	if relax {
		tableName = "rx"
	}
	cl := clanLeaderboard{Page: page}
	q := strings.Replace("SELECT SUM(pp_DBMODE)/(COUNT(clan_id)+1) AS pp, SUM(ranked_score_DBMODE), SUM(total_score_DBMODE), SUM(playcount_DBMODE), AVG(avg_accuracy_DBMODE), clans.name, clans.id FROM "+tableName+"_stats LEFT JOIN users ON users.id = "+tableName+"_stats.id INNER JOIN clans ON clans.id=users.clan_id WHERE clan_id <> 0 AND (users.privileges&3)>=3 GROUP BY clan_id ORDER BY pp DESC LIMIT ?,50", "DBMODE", dbmode[mode], -1)
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
		rank := i + (page-1)*50
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

var dbmode = [...]string{"std", "taiko", "ctb", "mania"}

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
		return Res{Clan: cms}
	}
	q := strings.Replace("SELECT SUM(pp_DBMODE)/(COUNT(clan_id)+1) AS pp, SUM(ranked_score_DBMODE), SUM(total_score_DBMODE), SUM(playcount_DBMODE), SUM(replays_watched_DBMODE), AVG(avg_accuracy_DBMODE), SUM(total_hits_DBMODE) FROM "+tableName+"_stats LEFT JOIN users ON users.id = "+tableName+"_stats.id WHERE users.clan_id = ? AND (users.privileges & 3) >= 3 LIMIT 1", "DBMODE", dbmode[mode], -1)
	var pp float64
	err = md.DB.QueryRow(q, id).Scan(&pp, &cms.ChosenMode.RankedScore, &cms.ChosenMode.TotalScore, &cms.ChosenMode.PlayCount, &cms.ChosenMode.ReplaysWatched, &cms.ChosenMode.Accuracy, &cms.ChosenMode.TotalHits)
	if err != nil {
		md.Err(err)
		return Err500
	}

	cms.ChosenMode.PP = int(pp)
	var rank int
	err = md.DB.QueryRow("SELECT COUNT(pp) FROM (SELECT SUM(pp_"+dbmode[mode]+")/(COUNT(clan_id)+1) AS pp FROM "+tableName+"_stats LEFT JOIN users ON users.id = "+tableName+"_stats.id WHERE clan_id <> 0 AND (users.privileges&3)>=3 GROUP BY clan_id) x WHERE x.pp >= ?", cms.ChosenMode.PP).Scan(&rank)
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

func resolveInvite(c string, md *common.MethodData) (id int, err error) {
	row := md.DB.QueryRow("SELECT id FROM clans where invite = ?", c)
	err = row.Scan(&id)

	return
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
		ID     int    `json:"id,omitempty"`
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
	var hasInvite bool

	if u.Invite != "" {
		u.ID, err = resolveInvite(u.Invite, &md)
		
		if err != nil {
			if err == sql.ErrNoRows {
				return common.SimpleResponse(404, "invalid invite provided")
			}
			md.Err(err)
			return Err500
		}
		
		hasInvite = true
	}
	
	if u.ID > 0 {	
		c, err := getClan(u.ID, md)
		if err != nil {
			if err == sql.ErrNoRows {
				return common.SimpleResponse(404, "clan not found")
			}
			md.Err(err)
			return Err500
		}
		
		if c.Status == 0 || (c.Status == 2 && !hasInvite) {
			return common.SimpleResponse(403, "closed")
		}
		
		var count int
		err = md.DB.QueryRow("SELECT COUNT(id) FROM users WHERE clan_id = ?", u.ID).Scan(&count)
		if err != nil {
			md.Err(err)
			return Err500
		}
		
		if count >= clanMemberLimit {
			return common.SimpleResponse(403, "clan is full")
		}
		
		_, err = md.DB.Exec("UPDATE users SET clan_id = ? WHERE id = ?", u.ID, md.ID())
		r.Clan = c
		r.Code = 200
		
		return r
	} else {
		return common.SimpleResponse(400, "invalid id parameter")
	}
}

func ClanLeavePOST(md common.MethodData) common.CodeMessager {
	if md.ID() == 0 {
		return common.SimpleResponse(401, "not authorised")
	}

	u := struct {
		ID int `json:"id"`
	}{}

	md.Unmarshal(&u)

	if u.ID <= 0 {
		return common.SimpleResponse(400, "invalid id")
	}

	c, err := getClan(u.ID, md)
	if err != nil {
		if err == sql.ErrNoRows {
			return common.SimpleResponse(404, "clan not found")
		}
		md.Err(err)
		return Err500
	}

	_, err = md.DB.Exec("UPDATE users SET clan_id = 0 WHERE id = ?", md.ID())
	if err != nil {
		md.Err(err)
		return Err500
	}

	var msg string
	if c.Owner == md.ID() {
		msg = "disbanded"
		_, err = md.DB.Exec("UPDATE users SET clan_id = 0 WHERE clan_id = ?", c.ID)
		if err != nil {
			md.Err(err)
			return Err500
		}
		_, err = md.DB.Exec("DELETE FROM clans WHERE id = ?", c.ID)
		if err != nil {
			md.Err(err)
			return Err500
		}
	}

	return common.SimpleResponse(200, msg)
}

func ClanSettingsPOST(md common.MethodData) common.CodeMessager {
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

	u := struct {
		Tag         string `json:"tag,omitempty"`
		Description string `json:"desc,omitempty"`
		Icon        string `json:"icon,omitempty"`
		Background  string `json"bg,omitempty"`
	}{}

	md.Unmarshal(&u)

	if len(u.Tag) > 6 || (len(u.Tag) > 0 && len(u.Tag) < 2) {
		return common.SimpleResponse(400, "invalid tag length")
	}

	// TODO: this should probably be an uploaded image to be safer..
	if u.Icon != "" {
		match, _ := regexp.MatchString(`^https?://(?:www\.)?.+\..+/.+\.(?:jpeg|jpg|png)/?$`, u.Icon)
		if !match {
			return common.SimpleResponse(200, "invalid icon url")
		}
	}

	if md.DB.QueryRow("SELECT 1 FROM clans WHERE tag = ?", u.Tag).Scan(new(int)) != sql.ErrNoRows {
		return common.SimpleResponse(200, "tag already exists")
	}

	i := make([]interface{}, 0) // probably a bad idea lol
	query := "UPDATE clans SET "
	if u.Tag != "" {
		query += "tag = ?"
		i = append(i, u.Tag)
	}
	if u.Description != "" {
		if len(i) != 0 {
			query += ", "
		}
		query += "description = ?"
		i = append(i, u.Description)
	}
	if u.Icon != "" {
		if len(i) != 0 {
			query += ", "
		}
		query += "icon = ?"
		i = append(i, u.Icon)
	}
	if u.Background != "" {
		if len(i) != 0 {
			query += ", "
		}
		query += "bg = ?"
		i = append(i, u.Background)
	}

	if len(i) == 0 {
		return common.SimpleResponse(400, "No fields filled.")
	}
	query += " WHERE id = ?"
	i = append(i, id)
	_, err = md.DB.Exec(query, i...)

	return common.SimpleResponse(200, "n")
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
	_, err = md.DB.Exec("UPDATE clans SET invite = ? WHERE id = ?", invite, id)
	if err != nil {
		md.Err(err)
		return Err500
	}

	r := struct {
		common.ResponseBase
		Invite string `json:"invite"`
	}{Invite: invite}
	r.Code = 200
	return r
}

func ClanKickPOST(md common.MethodData) common.CodeMessager {
	if md.ID() == 0 {
		return common.SimpleResponse(401, "not authorised")
	}

	var clan int
	if md.DB.QueryRow("SELECT id FROM clans WHERE owner = ?", md.ID()).Scan(&clan) == sql.ErrNoRows {
		return common.SimpleResponse(401, "not authorised")
	}

	u := struct {
		User int `json:"user"`
	}{}

	md.Unmarshal(&u)

	if u.User == 0 {
		return common.SimpleResponse(400, "bad user id")
	}

	/*if md.DB.QueryRow("SELECT 1 FROM users WHERE id = ? AND clan_id = ?", md.ID()).Scan(new(int)) == sql.ErrNoRows {
		return common.SimpleResponse(401, "not authorised")
	}*/

	_, err := md.DB.Exec("UPDATE users SET clan_id = 0 WHERE id = ? AND clan_id = ?", u.User, clan)
	if err != nil {
		md.Err(err)
		return Err500
	}

	return common.SimpleResponse(200, "success")
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

	rows, err := md.DB.Query(userFields+" WHERE users.privileges & 3 AND clan_id = ?", i)
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
	err := md.DB.QueryRow("SELECT id, name, description, tag, icon, owner, status FROM clans WHERE id = ? LIMIT 1", id).Scan(&c.ID, &c.Name, &c.Description, &c.Tag, &c.Icon, &c.Owner, &c.Status)

	return c, err
}

func getUserData(id int, md common.MethodData) (userData, error) {
	u := userData{}
	if id == 0 {
		return u, nil
	}
	err := md.DB.QueryRow("SELECT users.id, users.username, register_datetime, privileges, latest_activity, username_aka, country FROM users LEFT JOIN users_stats ON users.id = users_stats.id WHERE users.id = ? LIMIT 1", id).Scan(&u.ID, &u.Username, &u.RegisteredOn, &u.Privileges, &u.LatestActivity, &u.UsernameAKA, &u.Country)

	return u, err
}
