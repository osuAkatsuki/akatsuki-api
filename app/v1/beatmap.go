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

type beatmapSetStatusData struct {
	BeatmapsetID int `json:"beatmapset_id"`
	BeatmapID    int `json:"beatmap_id"`
	RankedStatus int `json:"ranked_status"`
	Frozen       int `json:"frozen"`
}

// BeatmapSetStatusPOST changes the ranked status of a beatmap, and whether
// the beatmap ranked status is frozen. Or freezed. Freezed best meme 2k16
func BeatmapSetStatusPOST(md common.MethodData) common.CodeMessager {
	var req beatmapSetStatusData
	md.Unmarshal(&req)

	var miss []string
	if req.BeatmapsetID <= 0 && req.BeatmapID <= 0 {
		miss = append(miss, "beatmapset_id or beatmap_id")
	}
	if len(miss) != 0 {
		return ErrMissingField(miss...)
	}

	if req.Frozen != 0 && req.Frozen != 1 {
		return common.SimpleResponse(400, "frozen status must be either 0 or 1")
	}
	if req.RankedStatus > 4 || -1 > req.RankedStatus {
		return common.SimpleResponse(400, "ranked status must be 5 < x < -2")
	}

	param := req.BeatmapsetID
	if req.BeatmapID != 0 {
		err := md.DB.QueryRow("SELECT beatmapset_id FROM beatmaps WHERE beatmap_id = ? LIMIT 1", req.BeatmapID).Scan(&param)
		switch {
		case err == sql.ErrNoRows:
			return common.SimpleResponse(404, "That beatmap could not be found!")
		case err != nil:
			md.Err(err)
			return Err500
		}
	}

	md.DB.Exec(`UPDATE beatmaps
		SET ranked = ?, ranked_status_freezed = ?
		WHERE beatmapset_id = ?`, req.RankedStatus, req.Frozen, param)

	if req.BeatmapID > 0 {
		md.Ctx.Request.URI().QueryArgs().SetUint("bb", req.BeatmapID)
	} else {
		md.Ctx.Request.URI().QueryArgs().SetUint("s", req.BeatmapsetID)
	}
	return getMultipleBeatmaps(md)
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

type beatmapReduced struct {
	BeatmapID          int    `json:"beatmap_id"`
	BeatmapsetID       int    `json:"beatmapset_id"`
	BeatmapMD5         string `json:"beatmap_md5"`
	Ranked             int    `json:"ranked"`
	RankedStatusFrozen int    `json:"ranked_status_frozen"`
}

type beatmapRankedFrozenFullResponse struct {
	common.ResponseBase
	Beatmaps []beatmapReduced `json:"beatmaps"`
}

// BeatmapRankedFrozenFullGET retrieves all beatmaps with a certain
// ranked_status_freezed
func BeatmapRankedFrozenFullGET(md common.MethodData) common.CodeMessager {
	rows, err := md.DB.Query(`
	SELECT beatmap_id, beatmapset_id, beatmap_md5, ranked, ranked_status_freezed
	FROM beatmaps
	WHERE ranked_status_freezed = '1'
	`)
	if err != nil {
		md.Err(err)
		return Err500
	}
	var r beatmapRankedFrozenFullResponse
	for rows.Next() {
		var b beatmapReduced
		err = rows.Scan(&b.BeatmapID, &b.BeatmapsetID, &b.BeatmapMD5, &b.Ranked, &b.RankedStatusFrozen)
		if err != nil {
			md.Err(err)
			continue
		}
		r.Beatmaps = append(r.Beatmaps, b)
	}
	r.Code = 200
	return r
}
