package peppy

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/osuAkatsuki/akatsuki-api/common"

	"github.com/jmoiron/sqlx"
	"github.com/valyala/fasthttp"
	"gopkg.in/thehowl/go-osuapi.v1"
	"zxq.co/x/getrank"
)

// GetScores retrieve information about the top 100 scores of a specified beatmap.
func GetScores(c *fasthttp.RequestCtx, db *sqlx.DB) {
	if query(c, "b") == "" {
		json(c, 200, defaultResponse)
		return
	}
	var beatmapMD5 string
	err := db.Get(&beatmapMD5, "SELECT beatmap_md5 FROM beatmaps WHERE beatmap_id = ? LIMIT 1", query(c, "b"))
	switch {
	case err == sql.ErrNoRows:
		json(c, 200, defaultResponse)
		return
	case err != nil:
		common.Err(c, err)
		json(c, 200, defaultResponse)
		return
	}

	rx := query(c, "rx")

	table := "scores"
	switch rx {
	case "1", "true", "True":
		table = "scores_relax"
	}
	fmt.Println(table)
	var sb = table + ".score"
	if rankable(query(c, "m")) {
		sb = table + ".pp"
	}
	var (
		extraWhere  string
		extraParams []interface{}
	)
	if query(c, "u") != "" {
		w, p := genUser(c, db)
		extraWhere = "AND " + w
		extraParams = append(extraParams, p)
	}
	mods := common.Int(query(c, "mods"))
	rows, err := db.Query(fmt.Sprintf(`
SELECT
	%[1]s.id, %[1]s.score, users.username, %[1]s.300_count, %[1]s.100_count,
	%[1]s.50_count, %[1]s.misses_count, %[1]s.gekis_count, %[1]s.katus_count,
	%[1]s.max_combo, %[1]s.full_combo, %[1]s.mods, users.id, %[1]s.time, %[1]s.pp,
	%[1]s.accuracy
FROM %[1]s
INNER JOIN users ON users.id = %[1]s.userid
WHERE %[1]s.completed = '3'
  AND users.privileges & 1 > 0
  AND %[1]s.beatmap_md5 = ?
  AND %[1]s.play_mode = ?
  AND %[1]s.mods & ? = ?
  `+extraWhere+`
ORDER BY `+sb+` DESC LIMIT `+strconv.Itoa(common.InString(1, query(c, "limit"), 100, 50)), table),
		append([]interface{}{beatmapMD5, genmodei(query(c, "m")), mods, mods}, extraParams...)...)
	if err != nil {
		common.Err(c, err)
		json(c, 200, defaultResponse)
		return
	}
	var results []osuapi.GSScore
	for rows.Next() {
		var (
			s         osuapi.GSScore
			fullcombo bool
			mods      int
			date      common.UnixTimestamp
			accuracy  float64
		)
		err := rows.Scan(
			&s.ScoreID, &s.Score.Score, &s.Username, &s.Count300, &s.Count100,
			&s.Count50, &s.CountMiss, &s.CountGeki, &s.CountKatu,
			&s.MaxCombo, &fullcombo, &mods, &s.UserID, &date, &s.PP,
			&accuracy,
		)
		if err != nil {
			if err != sql.ErrNoRows {
				common.Err(c, err)
			}
			continue
		}
		s.FullCombo = osuapi.OsuBool(fullcombo)
		s.Mods = osuapi.Mods(mods)
		s.Date = osuapi.MySQLDate(date)
		s.Rank = strings.ToUpper(getrank.GetRank(osuapi.Mode(genmodei(query(c, "m"))), s.Mods,
			accuracy, s.Count300, s.Count100, s.Count50, s.CountMiss))
		results = append(results, s)
	}
	json(c, 200, results)
	return
}
