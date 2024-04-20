package peppy

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/osuAkatsuki/akatsuki-api/common"
	"github.com/thehowl/go-osuapi"
	"github.com/valyala/fasthttp"
)

// GetBeatmap retrieves general beatmap information.
func GetBeatmap(c *fasthttp.RequestCtx, db *sqlx.DB) {
	var whereClauses []string
	var params []interface{}
	limit := strconv.Itoa(common.InString(1, query(c, "limit"), 500, 500))
	args := c.QueryArgs()

	// since value is not stored, silently ignore
	if args.Has("s") {
		whereClauses = append(whereClauses, "beatmaps.beatmapset_id = ?")
		params = append(params, query(c, "s"))
	}

	if args.Has("b") {
		whereClauses = append(whereClauses, "beatmaps.beatmap_id = ?")
		params = append(params, query(c, "b"))
		// b is unique, so change limit to 1
		limit = "1"
	}

	if args.Has("h") {
		whereClauses = append(whereClauses, "beatmaps.beatmap_md5 = ?")
		params = append(params, query(c, "h"))
	}

	where := strings.Join(whereClauses, " AND ")
	if where != "" {
		where = "WHERE " + where
	}

	rows, err := db.Query(`SELECT
	beatmapset_id, beatmap_id, ranked, hit_length,
	song_name, beatmap_md5, ar, od, bpm, playcount,
	passcount, max_combo, latest_update

FROM beatmaps `+where+" ORDER BY beatmap_id DESC LIMIT "+limit,
		params...)
	if err != nil {
		common.Err(c, err)
		json(c, 200, defaultResponse)
		return
	}

	var bms []osuapi.Beatmap
	for rows.Next() {
		var (
			bm              osuapi.Beatmap
			rawRankedStatus int
			rawName         string
			rawLastUpdate   common.UnixTimestamp
		)
		err := rows.Scan(
			&bm.BeatmapSetID, &bm.BeatmapID, &rawRankedStatus, &bm.HitLength,
			&rawName, &bm.FileMD5, &bm.ApproachRate, &bm.OverallDifficulty, &bm.BPM, &bm.Playcount,
			&bm.Passcount, &bm.MaxCombo, &rawLastUpdate,
		)
		if err != nil {
			common.Err(c, err)
			continue
		}
		bm.TotalLength = bm.HitLength
		bm.LastUpdate = osuapi.MySQLDate(rawLastUpdate)
		if rawRankedStatus >= 2 {
			bm.ApprovedDate = osuapi.MySQLDate(rawLastUpdate)
		}
		// zero value of ApprovedStatus == osuapi.StatusPending, so /shrug
		bm.Approved = rippleToOsuRankedStatus[rawRankedStatus]
		bm.Artist, bm.Title, bm.DiffName = parseDiffName(rawName)
		bms = append(bms, bm)
	}

	json(c, 200, bms)
}

var rippleToOsuRankedStatus = map[int]osuapi.ApprovedStatus{
	0: osuapi.StatusPending,
	1: osuapi.StatusWIP, // it means "needs updating", as the one in the db needs to be updated, but whatever
	2: osuapi.StatusRanked,
	3: osuapi.StatusApproved,
	4: osuapi.StatusQualified,
	5: osuapi.StatusLoved,
}

func parseDiffName(name string) (author string, title string, diffName string) {
	regex := regexp.MustCompile(`^(.*) - (.*) \[(.*)\]$`)
	matches := regex.FindStringSubmatch(name)

	if len(matches) != 4 {
		return
	}

	author = matches[1]
	title = matches[2]
	diffName = matches[3]

	return
}
