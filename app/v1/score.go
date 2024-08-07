package v1

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/osuAkatsuki/akatsuki-api/common"
	"gopkg.in/thehowl/go-osuapi.v1"
	"zxq.co/x/getrank"
)

// Score is a score done on Ripple.
type Score struct {
	ID         string               `json:"id,int64"`
	BeatmapMD5 string               `json:"beatmap_md5"`
	Score      int64                `json:"score"`
	MaxCombo   int                  `json:"max_combo"`
	FullCombo  bool                 `json:"full_combo"`
	Mods       int                  `json:"mods"`
	Count300   int                  `json:"count_300"`
	Count100   int                  `json:"count_100"`
	Count50    int                  `json:"count_50"`
	CountGeki  int                  `json:"count_geki"`
	CountKatu  int                  `json:"count_katu"`
	CountMiss  int                  `json:"count_miss"`
	Time       common.UnixTimestamp `json:"time"`
	PlayMode   int                  `json:"play_mode"`
	Accuracy   float64              `json:"accuracy"`
	PP         float32              `json:"pp"`
	Rank       string               `json:"rank"`
	Completed  int                  `json:"completed"`
	Pinned     bool                 `json:"pinned"`
	UserID     int                  `json:"user_id"`
}

// beatmapScore is to differentiate from userScore, as beatmapScore contains
// also an user, while userScore contains the beatmap.
type beatmapScore struct {
	Score
	User userData `json:"user"`
}

type scoresResponse struct {
	common.ResponseBase
	Scores []beatmapScore `json:"scores"`
}

type scoreResponse struct {
	common.ResponseBase
	Score   beatmapScore `json:"score"`
	Beatmap beatmap      `json:"beatmap"`
}

var vnQuery = `
SELECT
	scores.id, scores.beatmap_md5, scores.score,
	scores.max_combo, scores.full_combo, scores.mods,
	scores.300_count, scores.100_count, scores.50_count,
	scores.gekis_count, scores.katus_count, scores.misses_count,
	scores.time, scores.play_mode, scores.accuracy, scores.pp,
	scores.completed, scores.pinned, scores.userid,

	users.id, users.username, users.register_datetime, users.privileges,
	users.latest_activity, users.username_aka, users.country
FROM scores
INNER JOIN users ON users.id = scores.userid
WHERE scores.beatmap_md5 = ? AND scores.play_mode = ? AND scores.completed = '3' AND `

var rxQuery = `
SELECT
	scores_relax.id, scores_relax.beatmap_md5, scores_relax.score,
	scores_relax.max_combo, scores_relax.full_combo, scores_relax.mods,
	scores_relax.300_count, scores_relax.100_count, scores_relax.50_count,
	scores_relax.gekis_count, scores_relax.katus_count, scores_relax.misses_count,
	scores_relax.time, scores_relax.play_mode, scores_relax.accuracy, scores_relax.pp,
	scores_relax.completed, scores_relax.pinned, scores_relax.userid,

	users.id, users.username, users.register_datetime, users.privileges,
	users.latest_activity, users.username_aka, users.country
FROM scores_relax
INNER JOIN users ON users.id = scores_relax.userid
WHERE scores_relax.beatmap_md5 = ? AND scores_relax.play_mode = ? AND scores_relax.completed = '3' AND `

var apQuery = `
SELECT
	scores_ap.id, scores_ap.beatmap_md5, scores_ap.score,
	scores_ap.max_combo, scores_ap.full_combo, scores_ap.mods,
	scores_ap.300_count, scores_ap.100_count, scores_ap.50_count,
	scores_ap.gekis_count, scores_ap.katus_count, scores_ap.misses_count,
	scores_ap.time, scores_ap.play_mode, scores_ap.accuracy, scores_ap.pp,
	scores_ap.completed, scores_ap.pinned, scores_ap.userid,

	users.id, users.username, users.register_datetime, users.privileges,
	users.latest_activity, users.username_aka, users.country
FROM scores_ap
INNER JOIN users ON users.id = scores_ap.userid
WHERE scores_ap.beatmap_md5 = ? AND scores_ap.play_mode = ? AND scores_ap.completed = '3' AND `

var vnQuerySingle = `
SELECT
	scores.id, scores.beatmap_md5, scores.score,
	scores.max_combo, scores.full_combo, scores.mods,
	scores.300_count, scores.100_count, scores.50_count,
	scores.gekis_count, scores.katus_count, scores.misses_count,
	scores.time, scores.play_mode, scores.accuracy, scores.pp,
	scores.completed, scores.pinned, scores.userid,

	users.id, users.username, users.register_datetime, users.privileges,
	users.latest_activity, users.username_aka, users.country,

	beatmaps.beatmap_id, beatmaps.beatmapset_id, beatmaps.beatmap_md5,
	beatmaps.song_name, beatmaps.ar, beatmaps.od,
	beatmaps.max_combo, beatmaps.hit_length, beatmaps.ranked,
	beatmaps.ranked_status_freezed, beatmaps.latest_update
FROM scores
INNER JOIN users ON users.id = scores.userid
INNER JOIN beatmaps ON beatmaps.beatmap_md5 = scores.beatmap_md5
WHERE scores.id = ? `

var rxQuerySingle = `
SELECT
	scores_relax.id, scores_relax.beatmap_md5, scores_relax.score,
	scores_relax.max_combo, scores_relax.full_combo, scores_relax.mods,
	scores_relax.300_count, scores_relax.100_count, scores_relax.50_count,
	scores_relax.gekis_count, scores_relax.katus_count, scores_relax.misses_count,
	scores_relax.time, scores_relax.play_mode, scores_relax.accuracy, scores_relax.pp,
	scores_relax.completed, scores_relax.pinned, scores_relax.userid,

	users.id, users.username, users.register_datetime, users.privileges,
	users.latest_activity, users.username_aka, users.country,

	beatmaps.beatmap_id, beatmaps.beatmapset_id, beatmaps.beatmap_md5,
	beatmaps.song_name, beatmaps.ar, beatmaps.od,
	beatmaps.max_combo, beatmaps.hit_length, beatmaps.ranked,
	beatmaps.ranked_status_freezed, beatmaps.latest_update
FROM scores_relax
INNER JOIN users ON users.id = scores_relax.userid
INNER JOIN beatmaps ON beatmaps.beatmap_md5 = scores_relax.beatmap_md5
WHERE scores_relax.id = ? `

var apQuerySingle = `
SELECT
	scores_ap.id, scores_ap.beatmap_md5, scores_ap.score,
	scores_ap.max_combo, scores_ap.full_combo, scores_ap.mods,
	scores_ap.300_count, scores_ap.100_count, scores_ap.50_count,
	scores_ap.gekis_count, scores_ap.katus_count, scores_ap.misses_count,
	scores_ap.time, scores_ap.play_mode, scores_ap.accuracy, scores_ap.pp,
	scores_ap.completed, scores_ap.pinned, scores_ap.userid,

	users.id, users.username, users.register_datetime, users.privileges,
	users.latest_activity, users.username_aka, users.country,

	beatmaps.beatmap_id, beatmaps.beatmapset_id, beatmaps.beatmap_md5,
	beatmaps.song_name, beatmaps.ar, beatmaps.od,
	beatmaps.max_combo, beatmaps.hit_length, beatmaps.ranked,
	beatmaps.ranked_status_freezed, beatmaps.latest_update
FROM scores_ap
INNER JOIN users ON users.id = scores_ap.userid
INNER JOIN beatmaps ON beatmaps.beatmap_md5 = scores_ap.beatmap_md5
WHERE scores_ap.id = ? `

func ScoreGET(md common.MethodData) common.CodeMessager {
	relax := common.Int(md.Query("rx"))
	scoreId := md.Query("id")

	query := vnQuerySingle
	if relax == 1 {
		query = rxQuerySingle
	} else if relax == 2 {
		query = apQuerySingle
	}

	var (
		s beatmapScore
		u userData
		b beatmap
	)
	row := md.DB.QueryRow(query, scoreId)
	err := row.Scan(
		&s.ID, &s.BeatmapMD5, &s.Score.Score,
		&s.MaxCombo, &s.FullCombo, &s.Mods,
		&s.Count300, &s.Count100, &s.Count50,
		&s.CountGeki, &s.CountKatu, &s.CountMiss,
		&s.Time, &s.PlayMode, &s.Accuracy, &s.PP,
		&s.Completed, &s.Pinned, &s.UserID,

		&u.ID, &u.Username, &u.RegisteredOn, &u.Privileges,
		&u.LatestActivity, &u.UsernameAKA, &u.Country,

		&b.BeatmapID, &b.BeatmapsetID, &b.BeatmapMD5,
		&b.SongName, &b.AR, &b.OD,
		&b.MaxCombo, &b.HitLength, &b.Ranked,
		&b.RankedStatusFrozen, &b.LatestUpdate,
	)
	if err != nil {
		md.Err(err)
	}
	s.User = u
	s.Rank = strings.ToUpper(getrank.GetRank(
		osuapi.Mode(s.PlayMode),
		osuapi.Mods(s.Mods),
		s.Accuracy,
		s.Count300,
		s.Count100,
		s.Count50,
		s.CountMiss,
	))

	var r scoreResponse
	r.Score = s
	r.Beatmap = b
	r.Code = 200
	return r
}

// ScoresGET retrieves the top scores for a certain beatmap.
func ScoresGET(md common.MethodData) common.CodeMessager {
	var (
		beatmapMD5 string
		r          scoresResponse
	)
	switch {
	case md.Query("md5") != "":
		beatmapMD5 = md.Query("md5")
	case md.Query("b") != "":
		err := md.DB.Get(&beatmapMD5, "SELECT beatmap_md5 FROM beatmaps WHERE beatmap_id = ? LIMIT 1", md.Query("b"))
		switch {
		case err == sql.ErrNoRows:
			r.Code = 200
			return r
		case err != nil:
			md.Err(err)
			return Err500
		}
	default:
		return ErrMissingField("md5|b")
	}

	queryDb := vnQuery
	mc := genModeClause(md, true)
	sort := common.Sort(md, common.SortConfiguration{
		Default: "scores.pp DESC, scores.score DESC",
		Table:   "scores",
		Allowed: []string{"pp", "score", "accuracy", "id"},
	})

	if md.Query("relax") == "1" {
		queryDb = rxQuery
		mc = strings.Replace(mc, "scores.", "scores_relax.", 1)
		sort = common.Sort(md, common.SortConfiguration{
			Default: "scores_relax.pp DESC, scores_relax.score DESC",
			Table:   "scores_relax",
			Allowed: []string{"pp", "score", "accuracy", "id"},
		})
	} else if md.Query("relax") == "2" {
		queryDb = apQuery
		mc = strings.Replace(mc, "scores.", "scores_ap.", 1)
		sort = common.Sort(md, common.SortConfiguration{
			Default: "scores_ap.pp DESC, scores_ap.score DESC",
			Table:   "scores_ap",
			Allowed: []string{"pp", "score", "accuracy", "id"},
		})
	}
	mode := "0"
	if md.Query("m") != "" {
		mode = md.Query("m")
	}

	rows, err := md.DB.Query(queryDb+``+md.User.OnlyUserPublic(false)+
		` `+mc+` `+sort+common.Paginate(md.Query("p"), md.Query("l"), 100), beatmapMD5, mode)
	if err != nil {
		md.Err(err)
		return Err500
	}
	for rows.Next() {
		var (
			s beatmapScore
			u userData
		)
		err := rows.Scan(
			&s.ID, &s.BeatmapMD5, &s.Score.Score,
			&s.MaxCombo, &s.FullCombo, &s.Mods,
			&s.Count300, &s.Count100, &s.Count50,
			&s.CountGeki, &s.CountKatu, &s.CountMiss,
			&s.Time, &s.PlayMode, &s.Accuracy, &s.PP,
			&s.Completed, &s.Pinned, &s.UserID,

			&u.ID, &u.Username, &u.RegisteredOn, &u.Privileges,
			&u.LatestActivity, &u.UsernameAKA, &u.Country,
		)
		if err != nil {
			md.Err(err)
			continue
		}
		s.User = u
		s.Rank = strings.ToUpper(getrank.GetRank(
			osuapi.Mode(s.PlayMode),
			osuapi.Mods(s.Mods),
			s.Accuracy,
			s.Count300,
			s.Count100,
			s.Count50,
			s.CountMiss,
		))
		r.Scores = append(r.Scores, s)
	}
	r.Code = 200
	return r
}

func getMode(m string) string {
	switch m {
	case "1":
		return "taiko"
	case "2":
		return "ctb"
	case "3":
		return "mania"
	default:
		return "std"
	}
}

func getModeInt(m string) int {
	switch m {
	case "taiko":
		return 1
	case "ctb":
		return 2
	case "mania":
		return 3
	default:
		return 0
	}
}

func genModeClause(md common.MethodData, includeAnd bool) string {
	var modeClause string
	if md.Query("mode") != "" {
		m, err := strconv.Atoi(md.Query("mode"))
		if err == nil && m >= 0 && m <= 3 {
			modeClause = fmt.Sprintf("scores.play_mode = '%d'", m)

			if includeAnd {
				modeClause = "AND " + modeClause
			}
		}
	}
	return modeClause
}
