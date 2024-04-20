package v1

import (
	"database/sql"

	"github.com/osuAkatsuki/akatsuki-api/common"
)

type difficulty struct {
	STD   float64 `json:"std"`
	Taiko float64 `json:"taiko"`
	CTB   float64 `json:"ctb"`
	Mania float64 `json:"mania"`
}

type beatmap struct {
	BeatmapID    int     `json:"beatmap_id"`
	BeatmapsetID int     `json:"beatmapset_id"`
	BeatmapMD5   string  `json:"beatmap_md5"`
	SongName     string  `json:"song_name"`
	AR           float32 `json:"ar"`
	OD           float32 `json:"od"`

	// NOTE[2024-04-20]: We are removing the `difficulty_*` attributes
	// from the beatmaps database (in preference of using the new
	// performance-service or equivalent). We are leaving these
	// fields here in the API for backwards compatibility.
	Difficulty float64    `json:"difficulty"`
	Diff2      difficulty `json:"difficulty2"`

	MaxCombo           int                  `json:"max_combo"`
	HitLength          int                  `json:"hit_length"`
	Ranked             int                  `json:"ranked"`
	RankedStatusFrozen int                  `json:"ranked_status_frozen"`
	LatestUpdate       common.UnixTimestamp `json:"latest_update"`
}

type beatmapResponse struct {
	common.ResponseBase
	beatmap
}
type beatmapSetResponse struct {
	common.ResponseBase
	Beatmaps []beatmap `json:"beatmaps"`
}

// BeatmapGET retrieves a beatmap.
func BeatmapGET(md common.MethodData) common.CodeMessager {
	beatmapID := common.Int(md.Query("b"))
	if beatmapID != 0 {
		return getBeatmapSingle(md, beatmapID)
	}
	return getMultipleBeatmaps(md)
}

const baseBeatmapSelect = `
SELECT
	beatmap_id, beatmapset_id, beatmap_md5,
	song_name, ar, od, max_combo, hit_length,
	ranked, ranked_status_freezed, latest_update
FROM beatmaps
`

func getMultipleBeatmaps(md common.MethodData) common.CodeMessager {
	sort := common.Sort(md, common.SortConfiguration{
		Allowed: []string{
			"beatmapset_id",
			"beatmap_id",
			"ar",
			"od",
			"max_combo",
			"latest_update",
			"playcount",
			"passcount",
		},
		Default: "beatmap_id DESC",
		Table:   "beatmaps",
	})
	pm := md.Ctx.Request.URI().QueryArgs().PeekMulti
	where := common.
		Where("song_name = ?", md.Query("song_name")).
		Where("ranked_status_freezed = ?", md.Query("ranked_status_frozen"), "0", "1").
		In("beatmap_id", pm("bb")...).
		In("beatmapset_id", pm("s")...).
		In("beatmap_md5", pm("md5")...)

	rows, err := md.DB.Query(baseBeatmapSelect+
		where.Clause+" "+sort+" "+
		common.Paginate(md.Query("p"), md.Query("l"), 50), where.Params...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	var r beatmapSetResponse
	for rows.Next() {
		var b beatmap
		err = rows.Scan(
			&b.BeatmapID, &b.BeatmapsetID, &b.BeatmapMD5,
			&b.SongName, &b.AR, &b.OD, &b.MaxCombo,
			&b.HitLength, &b.Ranked, &b.RankedStatusFrozen,
			&b.LatestUpdate,
		)
		if err != nil {
			md.Err(err)
			continue
		}
		r.Beatmaps = append(r.Beatmaps, b)
	}
	r.Code = 200
	return r
}

func getBeatmapSingle(md common.MethodData, beatmapID int) common.CodeMessager {
	var b beatmap
	err := md.DB.QueryRow(baseBeatmapSelect+"WHERE beatmap_id = ? LIMIT 1", beatmapID).Scan(
		&b.BeatmapID, &b.BeatmapsetID, &b.BeatmapMD5,
		&b.SongName, &b.AR, &b.OD, &b.MaxCombo,
		&b.HitLength, &b.Ranked, &b.RankedStatusFrozen,
		&b.LatestUpdate,
	)
	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "That beatmap could not be found!")
	case err != nil:
		md.Err(err)
		return Err500
	}
	var r beatmapResponse
	r.Code = 200
	r.beatmap = b
	return r
}
