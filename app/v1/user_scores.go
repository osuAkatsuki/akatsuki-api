package v1

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/osuAkatsuki/akatsuki-api/common"
	"gopkg.in/thehowl/go-osuapi.v1"
	"zxq.co/x/getrank"
)

type userScore struct {
	Score
	Beatmap beatmap `json:"beatmap"`
}

type userScoresResponse struct {
	common.ResponseBase
	Scores []userScore `json:"scores"`
}

type pinResponse struct {
	common.ResponseBase
	ScoreId string `json:"score_id"`
}

const relaxScoreSelectBase = `
		SELECT
			scores_relax.id, scores_relax.beatmap_md5, scores_relax.score,
			scores_relax.max_combo, scores_relax.full_combo, scores_relax.mods,
			scores_relax.300_count, scores_relax.100_count, scores_relax.50_count,
			scores_relax.gekis_count, scores_relax.katus_count, scores_relax.misses_count,
			scores_relax.time, scores_relax.play_mode, scores_relax.accuracy, scores_relax.pp,
			scores_relax.completed, scores_relax.pinned,

			beatmaps.beatmap_id, beatmaps.beatmapset_id, beatmaps.beatmap_md5,
			beatmaps.song_name, beatmaps.ar, beatmaps.od,
			beatmaps.max_combo, beatmaps.hit_length, beatmaps.ranked,
			beatmaps.ranked_status_freezed, beatmaps.latest_update
		FROM scores_relax
		INNER JOIN beatmaps ON beatmaps.beatmap_md5 = scores_relax.beatmap_md5
		INNER JOIN users ON users.id = scores_relax.userid
		`

const autoScoreSelectBase = `
		SELECT
			scores_ap.id, scores_ap.beatmap_md5, scores_ap.score,
			scores_ap.max_combo, scores_ap.full_combo, scores_ap.mods,
			scores_ap.300_count, scores_ap.100_count, scores_ap.50_count,
			scores_ap.gekis_count, scores_ap.katus_count, scores_ap.misses_count,
			scores_ap.time, scores_ap.play_mode, scores_ap.accuracy, scores_ap.pp,
			scores_ap.completed, scores_ap.pinned,

			beatmaps.beatmap_id, beatmaps.beatmapset_id, beatmaps.beatmap_md5,
			beatmaps.song_name, beatmaps.ar, beatmaps.od,
			beatmaps.max_combo, beatmaps.hit_length, beatmaps.ranked,
			beatmaps.ranked_status_freezed, beatmaps.latest_update
		FROM scores_ap
		INNER JOIN beatmaps ON beatmaps.beatmap_md5 = scores_ap.beatmap_md5
		INNER JOIN users ON users.id = scores_ap.userid
		`

const userScoreSelectBase = `
		SELECT
			scores.id, scores.beatmap_md5, scores.score,
			scores.max_combo, scores.full_combo, scores.mods,
			scores.300_count, scores.100_count, scores.50_count,
			scores.gekis_count, scores.katus_count, scores.misses_count,
			scores.time, scores.play_mode, scores.accuracy, scores.pp,
			scores.completed, scores.pinned,

			beatmaps.beatmap_id, beatmaps.beatmapset_id, beatmaps.beatmap_md5,
			beatmaps.song_name, beatmaps.ar, beatmaps.od,
			beatmaps.max_combo, beatmaps.hit_length, beatmaps.ranked,
			beatmaps.ranked_status_freezed, beatmaps.latest_update
		FROM scores
		INNER JOIN beatmaps ON beatmaps.beatmap_md5 = scores.beatmap_md5
		INNER JOIN users ON users.id = scores.userid
		`

// UserScoresBestGET retrieves the best scores of an user, sorted by PP if
// mode is standard and sorted by ranked score otherwise.
func UserScoresBestGET(md common.MethodData) common.CodeMessager {
	cm, wc, param := whereClauseUser(md, "users")
	if cm != nil {
		return *cm
	}

	mc := genModeClause(md)
	rx := common.Int(md.Query("rx"))

	if rx == 1 {
		mc = strings.Replace(mc, "scores.", "scores_relax.", 1)
		return relaxPuts(md, fmt.Sprintf(
			`WHERE
				scores_relax.completed = 3
				AND beatmaps.ranked IN (2, 3)
				AND %s
				%s
				AND `+md.User.OnlyUserPublic(true)+`
			ORDER BY scores_relax.pp DESC, scores_relax.score DESC %s`,
			wc, mc, common.Paginate(md.Query("p"), md.Query("l"), 100),
		), param)
	} else if rx == 2 {
		mc = strings.Replace(mc, "scores.", "scores_ap.", 1)
		return autoPuts(md, fmt.Sprintf(
			`WHERE
				scores_ap.completed = 3
				AND beatmaps.ranked IN (2, 3)
				AND %s
				%s
				AND `+md.User.OnlyUserPublic(true)+`
			ORDER BY scores_ap.pp DESC, scores_ap.score DESC %s`,
			wc, mc, common.Paginate(md.Query("p"), md.Query("l"), 100),
		), param)
	} else {
		return scoresPuts(md, fmt.Sprintf(
			`WHERE
				scores.completed = 3
				AND beatmaps.ranked IN (2, 3)
				AND %s
				%s
				AND `+md.User.OnlyUserPublic(true)+`
			ORDER BY scores.pp DESC, scores.score DESC %s`,
			wc, mc, common.Paginate(md.Query("p"), md.Query("l"), 100),
		), param)
	}
}

// UserScoresRecentGET retrieves an user's latest scores.
func UserScoresRecentGET(md common.MethodData) common.CodeMessager {
	cm, wc, param := whereClauseUser(md, "users")
	if cm != nil {
		return *cm
	}
	mc := genModeClause(md)
	rx := common.Int(md.Query("rx"))

	if rx == 1 {
		mc = strings.Replace(mc, "scores.", "scores_relax.", 1)
		return relaxPuts(md, fmt.Sprintf(
			`WHERE
				%s
				%s
				AND time > UNIX_TIMESTAMP(NOW() - INTERVAL 24 HOUR)
				AND `+md.User.OnlyUserPublic(true)+`
			ORDER BY scores_relax.id DESC %s`,
			wc, mc, common.Paginate(md.Query("p"), md.Query("l"), 100),
		), param)
	} else if rx == 2 {
		mc = strings.Replace(mc, "scores.", "scores_ap.", 1)
		return autoPuts(md, fmt.Sprintf(
			`WHERE
				%s
				%s
				AND time > UNIX_TIMESTAMP(NOW() - INTERVAL 24 HOUR)
				AND `+md.User.OnlyUserPublic(true)+`
			ORDER BY scores_ap.id DESC %s`,
			wc, mc, common.Paginate(md.Query("p"), md.Query("l"), 100),
		), param)
	} else {
		return scoresPuts(md, fmt.Sprintf(
			`WHERE
				%s
				%s
				AND time > UNIX_TIMESTAMP(NOW() - INTERVAL 24 HOUR)
				AND `+md.User.OnlyUserPublic(true)+`
			ORDER BY scores.id DESC %s`,
			wc, mc, common.Paginate(md.Query("p"), md.Query("l"), 100),
		), param)
	}
}

// UserScoresPinnedGET retrieves an user's pinned scores.
func UserScoresPinnedGET(md common.MethodData) common.CodeMessager {
	cm, wc, param := whereClauseUser(md, "users")
	if cm != nil {
		return *cm
	}
	mc := genModeClause(md)
	rx := common.Int(md.Query("rx"))
	if rx == 1 {
		mc = strings.Replace(mc, "scores.", "scores_relax.", 1)
		return relaxPuts(md, fmt.Sprintf(
			`WHERE
				%s
				%s
				AND pinned = 1
				AND `+md.User.OnlyUserPublic(true)+`
			ORDER BY scores_relax.pp DESC %s`,
			wc, mc, common.Paginate(md.Query("p"), md.Query("l"), 100),
		), param)
	} else if rx == 2 {
		mc = strings.Replace(mc, "scores.", "scores_ap.", 1)
		return autoPuts(md, fmt.Sprintf(
			`WHERE
				%s
				%s
				AND pinned = 1
				AND `+md.User.OnlyUserPublic(true)+`
			ORDER BY scores_ap.pp DESC %s`,
			wc, mc, common.Paginate(md.Query("p"), md.Query("l"), 100),
		), param)
	} else {
		return scoresPuts(md, fmt.Sprintf(
			`WHERE
				%s
				%s
				AND pinned = 1
				AND `+md.User.OnlyUserPublic(true)+`
			ORDER BY scores.pp DESC %s`,
			wc, mc, common.Paginate(md.Query("p"), md.Query("l"), 100),
		), param)
	}
}

func ScoresPinAddPOST(md common.MethodData) common.CodeMessager {
	if md.ID() == 0 {
		return common.SimpleResponse(401, "not authorised")
	}

	var u struct {
		ID    string `json:"id"`
		Relax int    `json:"rx"`
	}
	md.Unmarshal(&u)

	id, err := strconv.ParseInt(u.ID, 10, 64)
	if err != nil {
		panic(err)
	}

	return pinScore(md, id, u.Relax, md.ID())
}

func ScoresPinDelPOST(md common.MethodData) common.CodeMessager {
	if md.ID() == 0 {
		return common.SimpleResponse(401, "not authorised")
	}

	var u struct {
		ID    string `json:"id"`
		Relax int    `json:"rx"`
	}
	md.Unmarshal(&u)

	id, err := strconv.ParseInt(u.ID, 10, 64)
	if err != nil {
		panic(err)
	}

	return unpinScore(md, id, u.Relax, md.ID())
}

func pinScore(md common.MethodData, id int64, relax int, userId int) common.CodeMessager {
	var table string
	if relax == 1 {
		table = "scores_relax"
	} else if relax == 2 {
		table = "scores_ap"
	} else {
		table = "scores"
	}

	var v int
	err := md.DB.QueryRow(fmt.Sprintf("SELECT userid FROM %s WHERE id = ?", table), id).Scan(&v)
	if err != nil {
		md.Err(err)
		return common.SimpleResponse(404, "I'd also like to pin a score I don't have... but I can't.")
	}
	if v != userId {
		return common.SimpleResponse(401, "no")
	}

	_, err = md.DB.Exec(fmt.Sprintf("UPDATE %s SET pinned = 1 WHERE id = ?", table), id)
	if err != nil {
		md.Err(err)
		return common.SimpleResponse(404, "I'd also like to pin a score I don't have... but I can't.")
	}

	r := pinResponse{}
	r.Code = 200
	r.ScoreId = strconv.FormatInt(id, 10)
	return r
}

func unpinScore(md common.MethodData, id int64, relax int, userId int) common.CodeMessager {
	var table string
	if relax == 1 {
		table = "scores_relax"
	} else if relax == 2 {
		table = "scores_ap"
	} else {
		table = "scores"
	}

	var v int
	err := md.DB.QueryRow(fmt.Sprintf("SELECT userid FROM %s WHERE id = ?", table), id).Scan(&v)
	if err != nil {
		md.Err(err)
		return common.SimpleResponse(404, "I'd also like to unpin a score I don't have... but I can't.")
	}
	if v != userId {
		return common.SimpleResponse(401, "no")
	}

	md.DB.Exec(fmt.Sprintf("UPDATE %s SET pinned = 0 WHERE id = ?", table), id)
	r := pinResponse{}
	r.Code = 200
	r.ScoreId = strconv.FormatInt(id, 10)
	return r
}

func scoresPuts(md common.MethodData, whereClause string, params ...interface{}) common.CodeMessager {
	rows, err := md.DB.Query(userScoreSelectBase+whereClause, params...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	var scores []userScore
	for rows.Next() {
		var (
			us userScore
			b  beatmap
		)
		err = rows.Scan(
			&us.ID, &us.BeatmapMD5, &us.Score.Score,
			&us.MaxCombo, &us.FullCombo, &us.Mods,
			&us.Count300, &us.Count100, &us.Count50,
			&us.CountGeki, &us.CountKatu, &us.CountMiss,
			&us.Time, &us.PlayMode, &us.Accuracy, &us.PP,
			&us.Completed, &us.Pinned,

			&b.BeatmapID, &b.BeatmapsetID, &b.BeatmapMD5,
			&b.SongName, &b.AR, &b.OD,
			&b.MaxCombo, &b.HitLength, &b.Ranked,
			&b.RankedStatusFrozen, &b.LatestUpdate,
		)
		if err != nil {
			md.Err(err)
			return Err500
		}
		us.Beatmap = b
		us.Rank = strings.ToUpper(getrank.GetRank(
			osuapi.Mode(us.PlayMode),
			osuapi.Mods(us.Mods),
			us.Accuracy,
			us.Count300,
			us.Count100,
			us.Count50,
			us.CountMiss,
		))
		scores = append(scores, us)
	}
	r := userScoresResponse{}
	r.Code = 200
	r.Scores = scores
	return r
}

func relaxPuts(md common.MethodData, whereClause string, params ...interface{}) common.CodeMessager {
	rows, err := md.DB.Query(relaxScoreSelectBase+whereClause, params...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	var scores []userScore
	for rows.Next() {
		var (
			us userScore
			b  beatmap
		)
		err = rows.Scan(
			&us.ID, &us.BeatmapMD5, &us.Score.Score,
			&us.MaxCombo, &us.FullCombo, &us.Mods,
			&us.Count300, &us.Count100, &us.Count50,
			&us.CountGeki, &us.CountKatu, &us.CountMiss,
			&us.Time, &us.PlayMode, &us.Accuracy, &us.PP,
			&us.Completed, &us.Pinned,

			&b.BeatmapID, &b.BeatmapsetID, &b.BeatmapMD5,
			&b.SongName, &b.AR, &b.OD,
			&b.MaxCombo, &b.HitLength, &b.Ranked,
			&b.RankedStatusFrozen, &b.LatestUpdate,
		)
		if err != nil {
			md.Err(err)
			return Err500
		}
		us.Beatmap = b
		us.Rank = strings.ToUpper(getrank.GetRank(
			osuapi.Mode(us.PlayMode),
			osuapi.Mods(us.Mods),
			us.Accuracy,
			us.Count300,
			us.Count100,
			us.Count50,
			us.CountMiss,
		))
		scores = append(scores, us)
	}
	r := userScoresResponse{}
	r.Code = 200
	r.Scores = scores
	return r
}

func autoPuts(md common.MethodData, whereClause string, params ...interface{}) common.CodeMessager {
	rows, err := md.DB.Query(autoScoreSelectBase+whereClause, params...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	var scores []userScore
	for rows.Next() {
		var (
			us userScore
			b  beatmap
		)
		err = rows.Scan(
			&us.ID, &us.BeatmapMD5, &us.Score.Score,
			&us.MaxCombo, &us.FullCombo, &us.Mods,
			&us.Count300, &us.Count100, &us.Count50,
			&us.CountGeki, &us.CountKatu, &us.CountMiss,
			&us.Time, &us.PlayMode, &us.Accuracy, &us.PP,
			&us.Completed, &us.Pinned,

			&b.BeatmapID, &b.BeatmapsetID, &b.BeatmapMD5,
			&b.SongName, &b.AR, &b.OD,
			&b.MaxCombo, &b.HitLength, &b.Ranked,
			&b.RankedStatusFrozen, &b.LatestUpdate,
		)
		if err != nil {
			md.Err(err)
			return Err500
		}
		us.Beatmap = b
		us.Rank = strings.ToUpper(getrank.GetRank(
			osuapi.Mode(us.PlayMode),
			osuapi.Mods(us.Mods),
			us.Accuracy,
			us.Count300,
			us.Count100,
			us.Count50,
			us.CountMiss,
		))
		scores = append(scores, us)
	}
	r := userScoresResponse{}
	r.Code = 200
	r.Scores = scores
	return r
}
