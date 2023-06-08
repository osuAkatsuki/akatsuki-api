package peppy

import (
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/osuAkatsuki/akatsuki-api/common"
	"github.com/valyala/fasthttp"
	"gopkg.in/thehowl/go-osuapi.v1"
	"zxq.co/x/getrank"
)

// GetUserRecent retrieves an user's recent scores.
func GetUserRecent(c *fasthttp.RequestCtx, db *sqlx.DB) {

	relax := query(c, "rx")
	if relax == "" {
		relax = "0"
	}

	rx := common.Int(relax)

	table := "scores"
	switch rx {
	case 1:
		table = "scores_relax"
	case 2:
		table = "scores_ap"
	}

	getUserX(c, db, "ORDER BY "+table+".time DESC", common.InString(1, query(c, "limit"), 50, 10))
}

// GetUserBest retrieves an user's best scores.
func GetUserBest(c *fasthttp.RequestCtx, db *sqlx.DB) {
	var sb string

	relax := query(c, "rx")
	if relax == "" {
		relax = "0"
	}

	rx := common.Int(relax)

	table := "scores"
	switch rx {
	case 1:
		table = "scores_relax"
	case 2:
		table = "scores_ap"
	}

	if rx != 0 {
		sb = table + ".pp"
	} else {
		sb = table + ".score"
	}
	getUserX(c, db, "AND completed = '3' ORDER BY "+sb+" DESC", common.InString(1, query(c, "limit"), 100, 10))
}

func getUserX(c *fasthttp.RequestCtx, db *sqlx.DB, orderBy string, limit int) {
	whereClause, p := genUser(c, db)

	relax := query(c, "rx")
	if relax == "" {
		relax = "0"
	}

	rx := common.Int(relax)

	table := "scores"
	switch rx {
	case 1:
		table = "scores_relax"
	case 2:
		table = "scores_ap"
	}

	sqlQuery := fmt.Sprintf(
		`SELECT
			beatmaps.beatmap_id, %[1]s.score, %[1]s.max_combo,
			%[1]s.300_count, %[1]s.100_count, %[1]s.50_count,
			%[1]s.gekis_count, %[1]s.katus_count, %[1]s.misses_count,
			%[1]s.full_combo, %[1]s.mods, users.id, %[1]s.time,
			%[1]s.pp, %[1]s.accuracy
		FROM %[1]s
		LEFT JOIN beatmaps ON beatmaps.beatmap_md5 = %[1]s.beatmap_md5
		LEFT JOIN users ON %[1]s.userid = users.id
		WHERE %[2]s AND %[1]s.play_mode = ? AND users.privileges & 1 > 0
		%[3]s
		LIMIT %[4]d`, table, whereClause, orderBy, limit,
	)
	scores := make([]osuapi.GUSScore, 0, limit)
	m := genmodei(query(c, "m"))
	rows, err := db.Query(sqlQuery, p, m)
	if err != nil {
		json(c, 200, defaultResponse)
		common.Err(c, err)
		return
	}
	for rows.Next() {
		var (
			curscore osuapi.GUSScore
			rawTime  common.UnixTimestamp
			acc      float64
			fc       bool
			mods     int
			bid      *int
		)
		err := rows.Scan(
			&bid, &curscore.Score.Score, &curscore.MaxCombo,
			&curscore.Count300, &curscore.Count100, &curscore.Count50,
			&curscore.CountGeki, &curscore.CountKatu, &curscore.CountMiss,
			&fc, &mods, &curscore.UserID, &rawTime,
			&curscore.PP, &acc,
		)
		if err != nil {
			json(c, 200, defaultResponse)
			common.Err(c, err)
			return
		}
		if bid == nil {
			curscore.BeatmapID = 0
		} else {
			curscore.BeatmapID = *bid
		}
		curscore.FullCombo = osuapi.OsuBool(fc)
		curscore.Mods = osuapi.Mods(mods)
		curscore.Date = osuapi.MySQLDate(rawTime)
		curscore.Rank = strings.ToUpper(getrank.GetRank(
			osuapi.Mode(m),
			curscore.Mods,
			acc,
			curscore.Count300,
			curscore.Count100,
			curscore.Count50,
			curscore.CountMiss,
		))
		scores = append(scores, curscore)
	}
	json(c, 200, scores)
}
