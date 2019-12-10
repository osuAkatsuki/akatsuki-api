package v1

import (
	// "database/sql"
	"zxq.co/ripple/rippleapi/common"
	"strconv"
	"strings"
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
	q := strings.Replace("SELECT SUM(pp_DBMODE)/(COUNT(clan)+1) AS pp, SUM(ranked_score_DBMODE), SUM(total_score_DBMODE), SUM(playcount_DBMODE), AVG(avg_accuracy_DBMODE), clans.name, clans.id FROM " + tableName + "_stats INNER JOIN clans ON clans.id=clan LEFT JOIN users ON users.id = " + tableName + "_stats.id WHERE clan <> 0 AND (users.privileges&3)>=3 GROUP BY clan ORDER BY pp DESC LIMIT ?,50", "DBMODE", dbmode[mode], -1)
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
	q := strings.Replace("SELECT SUM(pp_DBMODE)/(COUNT(clan)+1) AS pp, SUM(ranked_score_DBMODE), SUM(total_score_DBMODE), SUM(playcount_DBMODE), SUM(replays_watched_DBMODE), AVG(avg_accuracy_DBMODE), SUM(total_hits_DBMODE) FROM " + tableName + "_stats LEFT JOIN users ON users.id = " + tableName + "_stats.id WHERE clan = ? AND users.privileges & 3 LIMIT 1", "DBMODE", dbmode[mode], -1)
	var pp float64
	err = md.DB.QueryRow(q, id).Scan(&pp, &cms.ChosenMode.RankedScore, &cms.ChosenMode.TotalScore, &cms.ChosenMode.PlayCount, &cms.ChosenMode.ReplaysWatched, &cms.ChosenMode.Accuracy, &cms.ChosenMode.TotalHits)
	if err != nil {
		return Err500
	}
	
	cms.ChosenMode.PP = int(pp)
	var rank int
	err = md.DB.QueryRow("SELECT COUNT(*) FROM (SELECT SUM(pp_" + dbmode[mode] + ") / (1+(SELECT COUNT(id) FROM "+tableName+"_stats WHERE clan = clans.id)) AS pp FROM clans INNER JOIN " + tableName + "_stats ON " + tableName + "_stats.clan = clans.id GROUP BY clans.id) s WHERE s.pp >= ?", cms.ChosenMode.PP).Scan(&rank)
	if err != nil {
		return Err500
	}
	cms.ChosenMode.GlobalLeaderboardRank = &rank
	r := Res{Clan: cms}
	r.ResponseBase.Code = 200
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
		md.Err(err)
		return Err500
	}

	rows, err := md.DB.Query(userFields + " WHERE users.privileges & 3 AND clan = ?", i);
	if err != nil {
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
	type Res struct {
		common.ResponseBase
		Clan clanMembersData `json:"clan"`
	}
	res := Res{Clan: cmd}
	res.ResponseBase.Code = 200
	return res
}

func getClan(id int, md common.MethodData) (Clan, error) {
	c := Clan{}
	if id == 0 {
		return c, nil // lol?
	}
	err := md.DB.QueryRow("SELECT id, name, description, tag, icon, owner FROM clans WHERE id = ? LIMIT 1", md.Query("id")).Scan(&c.ID, &c.Name, &c.Description, &c.Tag, &c.Icon, &c.Owner)
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
