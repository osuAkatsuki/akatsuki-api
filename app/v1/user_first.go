// first version of rippleapi this package is only here to clean up shit because user.go is a mess
package v1

import (
	"fmt"
	"strings"

	"github.com/osuAkatsuki/akatsuki-api/common"
	"gopkg.in/thehowl/go-osuapi.v1"
	"zxq.co/x/getrank"
)

func UserFirstGET(md common.MethodData) common.CodeMessager {
	id := common.Int(md.Query("id"))
	if id == 0 {
		return ErrMissingField("id")
	}

	mode := common.Int(md.Query("mode"))

	rx := common.Int(md.Query("rx"))
	table := "scores"
	if rx == 1 {
		table = "scores_relax"
	} else if rx == 2 {
		table = "scores_ap"
	}

	r := struct {
		common.ResponseBase
		Total  int         `json:"total"`
		Scores []userScore `json:"scores"`
	}{}

	md.DB.Get(&r.Total, "SELECT COUNT(scoreid) FROM scores_first WHERE userid = ? AND mode = ? AND rx = ?", id, mode, rx)
	query := fmt.Sprintf(`SELECT
		%[1]s.id, %[1]s.beatmap_md5, %[1]s.score,
		%[1]s.max_combo, %[1]s.full_combo, %[1]s.mods,
		%[1]s.300_count, %[1]s.100_count, %[1]s.50_count,
		%[1]s.gekis_count, %[1]s.katus_count, %[1]s.misses_count,
		%[1]s.time, %[1]s.play_mode, %[1]s.accuracy, %[1]s.pp,
		%[1]s.completed,

		beatmaps.beatmap_id, beatmaps.beatmapset_id, beatmaps.beatmap_md5,
		beatmaps.song_name, beatmaps.ar, beatmaps.od,
		beatmaps.max_combo, beatmaps.hit_length, beatmaps.ranked,
		beatmaps.ranked_status_freezed, beatmaps.latest_update
		FROM scores_first
		INNER JOIN beatmaps ON beatmaps.beatmap_md5 = scores_first.beatmap_md5
		INNER JOIN %[1]s ON %[1]s.id = scores_first.scoreid WHERE scores_first.userid = ? AND scores_first.mode = ? AND scores_first.rx = ? ORDER BY %[1]s.time DESC %s`, table, common.Paginate(md.Query("p"), md.Query("l"), 100))

	rows, err := md.DB.Query(query, id, mode, rx)
	if err != nil {
		md.Err(err)
		return Err500
	}
	defer rows.Close()

	for rows.Next() {
		us := userScore{}
		err = rows.Scan(&us.ID, &us.BeatmapMD5, &us.Score.Score,
			&us.MaxCombo, &us.FullCombo, &us.Mods,
			&us.Count300, &us.Count100, &us.Count50,
			&us.CountGeki, &us.CountKatu, &us.CountMiss,
			&us.Time, &us.PlayMode, &us.Accuracy, &us.PP,
			&us.Completed,

			&us.Beatmap.BeatmapID, &us.Beatmap.BeatmapsetID, &us.Beatmap.BeatmapMD5,
			&us.Beatmap.SongName, &us.Beatmap.AR, &us.Beatmap.OD,
			&us.Beatmap.MaxCombo, &us.Beatmap.HitLength, &us.Beatmap.Ranked,
			&us.Beatmap.RankedStatusFrozen, &us.Beatmap.LatestUpdate)
		if err != nil {
			md.Err(err)
			return Err500
		}

		us.Rank = strings.ToUpper(getrank.GetRank(
			osuapi.Mode(us.PlayMode),
			osuapi.Mods(us.Mods),
			us.Accuracy,
			us.Count300,
			us.Count100,
			us.Count50,
			us.CountMiss,
		))

		r.Scores = append(r.Scores, us)
	}

	r.Code = 200
	return r
}
