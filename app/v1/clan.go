package v1

import (
	"database/sql"

	// "regexp"
	"strconv"
	"strings"

	"github.com/osuAkatsuki/akatsuki-api/common"
	"zxq.co/x/rs"
)

type Clan struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Tag         string `json:"tag"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Owner       int    `json:"owner"`
	Status      int    `json:"status"`
}

const (
	ClanClosed = iota
	ClanOpen
	ClanInviteOnly
	ClanRequestsOpen
)

const clanMemberLimit = 25

type SingleClanResponse struct {
	common.ResponseBase
	Clan `json:"clan"`
}

// clansGET retrieves all the clans on this ripple instance.
func ClansGET(md common.MethodData) common.CodeMessager {
	if md.Query("id") != "" {
		r := SingleClanResponse{}
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
	relax := common.Int(md.Query("rx"))
	if relax < 0 || relax > 2 {
		return common.SimpleResponse(400, "invalid relax value")
	}

	cl := clanLeaderboard{Page: page}
	q := `SELECT SUM(pp) / (COUNT(clan_id) + 1) AS pp, SUM(ranked_score),
	    SUM(total_score), SUM(playcount),
		AVG(avg_accuracy), clans.name, clans.id
		FROM user_stats
		LEFT JOIN users ON users.id = user_stats.user_id
		INNER JOIN clans ON clans.id = users.clan_id
		WHERE users.clan_id <> 0
		AND user_stats.mode = ?
		AND users.privileges & 1
		GROUP BY users.clan_id
		ORDER BY pp DESC
		LIMIT ?, 50`

	rows, err := md.DB.Query(q, mode+(relax*4), (page-1)*50)
	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()
	for i := 1; rows.Next(); i++ {
		clan := clanLbData{}
		var pp float64
		err = rows.Scan(&pp, &clan.ChosenMode.RankedScore, &clan.ChosenMode.TotalScore, &clan.ChosenMode.PlayCount, &clan.ChosenMode.Accuracy, &clan.Name, &clan.ID)
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

	relax := common.Int(md.Query("rx"))
	if relax < 0 || relax > 2 {
		return common.SimpleResponse(400, "invalid relax value")
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
	q := `SELECT SUM(pp) / (COUNT(users.clan_id) + 1) AS pp, SUM(ranked_score),
		SUM(total_score), SUM(playcount), SUM(replays_watched),
		AVG(avg_accuracy), SUM(total_hits)
		FROM user_stats LEFT JOIN users ON users.id = user_stats.user_id
		WHERE users.clan_id = ? AND user_stats.mode = ? AND users.privileges & 1`
	var pp float64
	err = md.DB.QueryRow(q, id, mode+(relax*4)).Scan(
		&pp, &cms.ChosenMode.RankedScore,
		&cms.ChosenMode.TotalScore, &cms.ChosenMode.PlayCount, &cms.ChosenMode.ReplaysWatched,
		&cms.ChosenMode.Accuracy, &cms.ChosenMode.TotalHits,
	)
	if err != nil {
		md.Err(err)
		return Err500
	}

	cms.ChosenMode.PP = int(pp)
	var rank int
	err = md.DB.QueryRow(`
		SELECT COUNT(pp)
		FROM (
			SELECT SUM(pp) / (COUNT(clan_id) + 1) AS pp
			FROM user_stats LEFT JOIN users ON users.id = user_stats.user_id
			WHERE clan_id <> 0
			AND user_stats.mode = ?
			AND (users.privileges & 3) >= 3
			GROUP BY clan_id
		) x
		WHERE x.pp >= ?`,
		mode+(relax*4),
		cms.ChosenMode.PP,
	).Scan(&rank)
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
	err := md.DB.QueryRow("SELECT id, name, description, tag, icon, owner FROM clans WHERE invite = ?", s).Scan(&clan.ID, &clan.Name, &clan.Description, &clan.Tag, &clan.Icon, &clan.Owner)
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
	u.Invite = strings.TrimSpace(u.Invite)
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
		err = md.DB.QueryRow("SELECT COUNT(id) FROM users WHERE clan_id = ?", c.ID).Scan(&count)
		if err != nil {
			md.Err(err)
			return Err500
		}

		if count >= clanMemberLimit {
			return common.SimpleResponse(403, "clan is full")
		}

		if c.Status == 3 {
			_, err = md.DB.Exec("INSERT INTO clan_requests VALUES (?, ?, DEFAULT) ON DUPLICATE KEY UPDATE time = NOW()", c.ID, md.ID())
			if err != nil {
				md.Err(err)
				return Err500
			}

			return common.SimpleResponse(200, "join request sent")
		}
		_, err = md.DB.Exec("UPDATE users SET clan_id = ? WHERE id = ?", c.ID, md.ID())
		if err != nil {
			md.Err(err)
			return Err500
		}

		r.Clan = c
		r.Code = 200

		md.R.Publish("api:update_user_clan", strconv.Itoa(md.ID()))

		return r
	} else {
		return common.SimpleResponse(400, "invalid id parameter")
	}
}

func ClanLeavePOST(md common.MethodData) common.CodeMessager {
	if md.ID() == 0 {
		return common.SimpleResponse(401, "not authorised")
	}

	clanId := 0
	row := md.DB.QueryRowx("SELECT clan_id FROM users WHERE id = ?", md.ID())
	if err := row.Scan(&clanId); err != nil {
		md.Err(err)
		return Err500
	}
	if clanId == 0 {
		return common.SimpleResponse(403, "You haven't joined any clan...")
	}
	clan, err := getClan(clanId, md)
	if err != nil {
		md.Err(err)
		return Err500
	}

	if clan.Owner == md.ID() {
		_, err = md.DB.Exec("UPDATE users SET clan_id = 0 WHERE clan_id = ?", clan.ID)
		if err != nil {
			md.Err(err)
			return Err500
		}

		err := disbandClan(clan.ID, md)
		if err != nil {
			md.Err(err)
			return Err500
		}
	} else {
		_, err := md.DB.Exec("UPDATE users SET clan_id = 0 WHERE id = ?", md.ID())
		if err != nil {
			md.Err(err)
			return Err500
		}
	}

	md.R.Publish("api:update_user_clan", strconv.Itoa(md.ID()))

	return common.SimpleResponse(200, "success")
}

func disbandClan(clanId int, md common.MethodData) error {
	_, err := md.DB.Exec("DELETE FROM clans WHERE id = ?", clanId)
	return err
}

func ClanSettingsPOST(md common.MethodData) common.CodeMessager {
	if md.ID() == 0 {
		return common.SimpleResponse(401, "not authorised")
	}

	var c Clan
	err := md.DB.QueryRow("SELECT id, tag, description, icon FROM clans WHERE owner = ?", md.ID()).Scan(&c.ID, &c.Tag, &c.Description, &c.Icon)
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
		// Icon        string `json:"icon,omitempty"`
		Background string `json:"bg,omitempty"`
		Status     int    `json:"status"`
	}{}

	md.Unmarshal(&u)
	u.Tag = strings.TrimSpace(u.Tag)

	// TODO: this should probably be an uploaded image to be safer..
	/* if u.Icon != "" {
		match, _ := regexp.MatchString(`^https?://(?:www\.)?.+\..+/.+\.(?:jpeg|jpg|png)/?$`, u.Icon)
		if !match {
			return common.SimpleResponse(200, "invalid icon url")
		}
	} */
	rss := []rune(u.Tag)
	if len(rss) > 6 || len(rss) < 1 {
		return common.SimpleResponse(400, "The given tag is too short or too long")
	} else if md.DB.QueryRow("SELECT 1 FROM clans WHERE tag = ? AND id != ?", u.Tag, c.ID).Scan(new(int)) != sql.ErrNoRows {
		return common.SimpleResponse(403, "Another Clan has already taken this Tag")
	}

	_, err = md.DB.Exec("UPDATE clans SET tag = ?, description = ?, background = ?, status = ? WHERE id = ?", u.Tag, u.Description, u.Background, u.Status, c.ID)

	if err != nil {
		md.Err(err)
		return Err500
	}

	md.R.Publish("api:update_clan", strconv.Itoa(c.ID))

	return SingleClanResponse{
		common.ResponseBase{
			Code: 200,
		},
		c,
	}
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

	md.R.Publish("api:update_user_clan", strconv.Itoa(md.ID()))

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

	cmd.Members = make([]userData, 0)
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
	err := md.DB.QueryRow("SELECT id, name, description, tag, icon, owner, status FROM clans WHERE id = ?", id).Scan(&c.ID, &c.Name, &c.Description, &c.Tag, &c.Icon, &c.Owner, &c.Status)

	return c, err
}
