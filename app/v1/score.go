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
	ID         int                  `json:"id"`
	BeatmapMD5 string               `json:"map_md5"`
	Score      int64                `json:"score"`
	MaxCombo   int                  `json:"max_combo"`
	FullCombo  bool                 `json:"perfect"`
	Mods       int                  `json:"mods"`
	Count300   int                  `json:"n300"`
	Count100   int                  `json:"n100"`
	Count50    int                  `json:"n50"`
	CountGeki  int                  `json:"ngeki"`
	CountKatu  int                  `json:"nkatu"`
	CountMiss  int                  `json:"nmiss"`
	Time       common.UnixTimestamp `json:"play_time"`
	PlayMode   int                  `json:"mode"`
	Accuracy   float64              `json:"acc"`
	PP         float32              `json:"pp"`
	Rank       string               `json:"rank"`
	Completed  int                  `json:"status"`
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
		err := md.DB.Get(&beatmapMD5, "SELECT md5 FROM maps WHERE id = ? LIMIT 1", md.Query("b"))
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

	sort := common.Sort(md, common.SortConfiguration{
		Default: "scores.pp DESC, scores.score DESC",
		Table:   "scores",
		Allowed: []string{"pp", "score", "accuracy", "id"},
	})

	rows, err := md.DB.Query(`
SELECT
	scores.id, scores.map_md5, scores.score,
	scores.max_combo, scores.perfect, scores.mods,
	scores.n300, scores.n100, scores.n50,
	scores.ngeki, scores.nkatu, scores.nmiss,
	scores.play_time, scores.mode, scores.accuracy, scores.pp,
	scores.status,

	users.id, users.username, users.register_datetime, users.privileges,
	users.latest_activity, users.username_aka, users.country
FROM scores
INNER JOIN users ON users.id = scores.userid
WHERE scores.map_md5 = ? AND scores.status = 2 AND `+md.User.OnlyUserPublic(false)+
		` `+genModeClause(md)+`
`+sort+common.Paginate(md.Query("p"), md.Query("l"), 100), beatmapMD5)
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
			&s.Completed,

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

func genModeClause(md common.MethodData) string {
	var modeClause string
	if md.Query("mode") != "" {
		m, err := strconv.Atoi(md.Query("mode"))
		if err == nil && m >= 0 && m <= 3 {
			modeClause = fmt.Sprintf("AND scores.mode = '%d'", m)
		}
	}
	return modeClause
}
