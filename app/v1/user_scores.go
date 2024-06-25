package v1

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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

// UserScoresBestGET retrieves the best scores of an user, sorted by PP if
// mode is standard and sorted by ranked score otherwise.
func UserScoresBestGET(md common.MethodData) common.CodeMessager {
	cm, wc, param := whereClauseUser(md, "users")
	if cm != nil {
		return *cm
	}

	mode := common.Int(md.Query("mode"))
	rx := common.Int(md.Query("rx"))

	query := fmt.Sprintf(`
		SELECT
			scores.id, scores.beatmap_md5, scores.score,
			scores.max_combo, scores.full_combo, scores.mods,
			scores.300_count, scores.100_count, scores.50_count,
			scores.gekis_count, scores.katus_count, scores.misses_count,
			scores.time, scores.play_mode, scores.accuracy, scores.pp,
			scores.completed, scores.pinned,

			beatmaps.beatmap_id, beatmaps.beatmapset_id, beatmaps.beatmap_md5 AS beatmap_beatmap_md5,
			beatmaps.song_name, beatmaps.ar, beatmaps.od,
			beatmaps.max_combo AS beatmap_max_combo, beatmaps.hit_length, beatmaps.ranked,
			beatmaps.ranked_status_freezed, beatmaps.latest_update
		FROM scores
		INNER JOIN beatmaps ON beatmaps.beatmap_md5 = scores.beatmap_md5
		INNER JOIN users ON users.id = scores.userid
		WHERE scores.completed = 3
		AND beatmaps.ranked IN (2, 3)
		AND %s
		AND %s
		AND play_mode = ?
		ORDER BY scores.pp DESC, scores.score DESC %s`,
		wc, md.User.OnlyUserPublic(true), common.Paginate(md.Query("p"), md.Query("l"), 100))

	if rx == 1 {
		query = strings.Replace(query, "scores", "scores_relax", -1)
	} else if rx == 2 {
		query = strings.Replace(query, "scores", "scores_ap", -1)
	}

	return scoresPuts(md, query, param, mode)
}

// UserScoresRecentGET retrieves an user's latest scores.
func UserScoresRecentGET(md common.MethodData) common.CodeMessager {
	cm, wc, param := whereClauseUser(md, "users")
	if cm != nil {
		return *cm
	}

	mode := common.Int(md.Query("mode"))
	rx := common.Int(md.Query("rx"))

	query := fmt.Sprintf(`
		SELECT
			scores.id, scores.beatmap_md5, scores.score,
			scores.max_combo, scores.full_combo, scores.mods,
			scores.300_count, scores.100_count, scores.50_count,
			scores.gekis_count, scores.katus_count, scores.misses_count,
			scores.time, scores.play_mode, scores.accuracy, scores.pp,
			scores.completed, scores.pinned,

			beatmaps.beatmap_id, beatmaps.beatmapset_id, beatmaps.beatmap_md5 AS beatmap_beatmap_md5,
			beatmaps.song_name, beatmaps.ar, beatmaps.od,
			beatmaps.max_combo AS beatmap_max_combo, beatmaps.hit_length, beatmaps.ranked,
			beatmaps.ranked_status_freezed, beatmaps.latest_update
		FROM scores
		INNER JOIN beatmaps ON beatmaps.beatmap_md5 = scores.beatmap_md5
		INNER JOIN users ON users.id = scores.userid
		AND %s
		AND %s
		AND play_mode = ?
		ORDER BY scores.id DESC %s`,
		wc, md.User.OnlyUserPublic(true), common.Paginate(md.Query("p"), md.Query("l"), 100))

	if rx == 1 {
		query = strings.Replace(query, "scores", "scores_relax", -1)
	} else if rx == 2 {
		query = strings.Replace(query, "scores", "scores_ap", -1)
	}

	response := scoresPuts(md, query, param, mode)

	if response.GetCode() != 200 {
		return response
	}

	scoresResponse := response.(userScoresResponse)

	scores := []userScore{}
	for i := range scoresResponse.Scores {
		if scoresResponse.Scores[i].Time.After(time.Now().Add(-24 * time.Hour)) {
			scores = append(scores, scoresResponse.Scores[i])
		}
	}

	scoresResponse.Scores = scores
	return scoresResponse
}

// UserScoresPinnedGET retrieves an user's pinned scores.
func UserScoresPinnedGET(md common.MethodData) common.CodeMessager {
	cm, wc, param := whereClauseUser(md, "users")
	if cm != nil {
		return *cm
	}

	mode := common.Int(md.Query("mode"))
	rx := common.Int(md.Query("rx"))

	query := fmt.Sprintf(`
		SELECT
			scores.id, scores.beatmap_md5, scores.score,
			scores.max_combo, scores.full_combo, scores.mods,
			scores.300_count, scores.100_count, scores.50_count,
			scores.gekis_count, scores.katus_count, scores.misses_count,
			scores.time, scores.play_mode, scores.accuracy, scores.pp,
			scores.completed, scores.pinned,

			beatmaps.beatmap_id, beatmaps.beatmapset_id, beatmaps.beatmap_md5 AS beatmap_beatmap_md5,
			beatmaps.song_name, beatmaps.ar, beatmaps.od,
			beatmaps.max_combo AS beatmap_max_combo, beatmaps.hit_length, beatmaps.ranked,
			beatmaps.ranked_status_freezed, beatmaps.latest_update
		FROM scores
		INNER JOIN beatmaps ON beatmaps.beatmap_md5 = scores.beatmap_md5
		INNER JOIN users ON users.id = scores.userid
		AND %s
		AND pinned = 1
		AND %s
		AND play_mode = ?
		ORDER BY scores.pp DESC %s`,
		wc, md.User.OnlyUserPublic(true), common.Paginate(md.Query("p"), md.Query("l"), 100))

	if rx == 1 {
		query = strings.Replace(query, "scores", "scores_relax", -1)
	} else if rx == 2 {
		query = strings.Replace(query, "scores", "scores_ap", -1)
	}

	return scoresPuts(md, query, param, mode)
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

func scoresPuts(md common.MethodData, query string, params ...interface{}) common.CodeMessager {
	rows, err := md.DB.Query(query, params...)
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
