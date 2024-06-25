package v1

import (
	"github.com/osuAkatsuki/akatsuki-api/common"
)

func ClansFirstPlaceRankingGET(md common.MethodData) common.CodeMessager {
	mode := common.Int(md.Query("m"))
	if mode > 3 {
		mode = 0
	}

	rx := common.Int(md.Query("rx"))
	rows, err := md.DB.Query(`
		SELECT COUNT(*) AS count, clans.id, clans.tag, clans.name
		FROM scores_first
		JOIN users ON users.id = userid
		JOIN clans ON clans.id = users.clan_id
		WHERE scores_first.mode = ?
		AND scores_first.rx = ?
		GROUP BY users.clan_id
		ORDER BY count DESC
		`+common.Paginate(md.Query("p"), md.Query("l"), 100),
		mode,
		rx,
	)
	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()
	type LbFirstEntry struct {
		Count  int    `json:"count"`
		ClanID int    `json:"clan"`
		Name   string `json:"name"`
		Tag    string `json:"tag"`
	}

	r := struct {
		common.ResponseBase
		Clans []LbFirstEntry `json:"clans"`
	}{}

	for rows.Next() {
		e := LbFirstEntry{}
		err = rows.Scan(&e.Count, &e.ClanID, &e.Tag, &e.Name)
		if err != nil {
			md.Err(err)
			return Err500
		}

		r.Clans = append(r.Clans, e)
	}

	r.Code = 200
	return r
}
