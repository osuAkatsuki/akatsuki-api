// Package beatmapget is an helper package to retrieve beatmap information from
// the osu! API, if the beatmap in the database is too old.
package beatmapget

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/osuAkatsuki/akatsuki-api/common"
	"gopkg.in/thehowl/go-osuapi.v1"
)

// Expire is the duration after which a beatmap expires.
const Expire = time.Hour * 12

// DB is the database.
var DB *sqlx.DB

// Client is the osu! api client to use
var Client *osuapi.Client

// BeatmapDefiningQuality is the defining quality of the beatmap to be updated,
// which is to say either the ID, the set ID or the md5 hash.
type BeatmapDefiningQuality struct {
	ID     int
	MD5    string
	frozen bool
	ranked int
}

func (b BeatmapDefiningQuality) String() string {
	if b.MD5 != "" {
		return "#/" + b.MD5
	}
	if b.ID != 0 {
		return "b/" + strconv.Itoa(b.ID)
	}
	return "n/a"
}

func (b BeatmapDefiningQuality) isSomethingSet() error {
	if b.ID == 0 && b.MD5 == "" {
		return errors.New("beatmapget: at least one field in BeatmapDefiningQuality must be set")
	}
	return nil
}

func (b BeatmapDefiningQuality) whereAndParams() (where string, params []interface{}) {
	var wheres []string
	if b.ID != 0 {
		wheres = append(wheres, "beatmap_id = ?")
		params = append(params, b.ID)
	}
	if b.MD5 != "" {
		wheres = append(wheres, "beatmap_md5 = ?")
		params = append(params, b.MD5)
	}
	where = strings.Join(wheres, " AND ")
	if where == "" {
		where = "1"
	}
	return
}

// UpdateIfRequired updates the beatmap in the database if an update is required.
func UpdateIfRequired(b BeatmapDefiningQuality) error {
	required, err := UpdateRequired(&b)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if !required {
		return nil
	}
	return Update(b, err != sql.ErrNoRows)
}

// UpdateRequired checks an update is required. If error is sql.ErrNoRows,
// it means that the beatmap should be updated, and that there was not the
// beatmap in the database previously.
func UpdateRequired(b *BeatmapDefiningQuality) (bool, error) {
	if err := b.isSomethingSet(); err != nil {
		return false, err
	}
	where, params := b.whereAndParams()
	var data struct {
		Difficulties [3]float64
		Ranked       int
		Frozen       bool
		LatestUpdate common.UnixTimestamp
	}
	err := DB.QueryRow(
		"SELECT ranked,ranked_status_freezed, latest_update "+
			"FROM beatmaps WHERE "+where+" LIMIT 1", params...).
		Scan(&data.Ranked, &data.Frozen, &data.LatestUpdate)
	b.frozen = data.Frozen
	if b.frozen {
		b.ranked = data.Ranked
	}
	if err != nil {
		if err == sql.ErrNoRows {
			return true, err
		}
		return false, err
	}

	expire := Expire
	if data.Ranked == 2 {
		expire *= 6
	}

	if expire != 0 && time.Now().After(time.Time(data.LatestUpdate).Add(expire)) && !data.Frozen {
		return true, nil
	}
	return false, nil
}

// Update updates a beatmap.
func Update(b BeatmapDefiningQuality, beatmapInDB bool) error {
	var data [4]osuapi.Beatmap
	for i := 0; i <= 3; i++ {
		mode := osuapi.Mode(i)
		beatmaps, err := Client.GetBeatmaps(osuapi.GetBeatmapsOpts{
			BeatmapID:   b.ID,
			BeatmapHash: b.MD5,
			Mode:        &mode,
		})
		if err != nil {
			return err
		}
		if len(beatmaps) == 0 {
			continue
		}
		data[i] = beatmaps[0]
	}
	var main *osuapi.Beatmap
	for _, el := range data {
		if el.FileMD5 != "" {
			main = &el
			break
		}
	}
	if main == nil {
		return fmt.Errorf("beatmapget: beatmap %s not found", b.String())
	}
	if beatmapInDB {
		w, p := b.whereAndParams()
		DB.MustExec("DELETE FROM beatmaps WHERE "+w, p...)
	}
	if b.frozen {
		main.Approved = osuapi.ApprovedStatus(b.ranked)
	}
	songName := fmt.Sprintf("%s - %s [%s]", main.Artist, main.Title, main.DiffName)
	_, err := DB.Exec(`INSERT INTO
	beatmaps (
		beatmap_id, beatmapset_id, beatmap_md5,
		song_name, ar, od, max_combo, hit_length,
		bpm, ranked, latest_update, ranked_status_freezed
	)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`,
		main.BeatmapID, main.BeatmapSetID, main.FileMD5,
		songName, main.ApproachRate, main.OverallDifficulty, main.MaxCombo, main.HitLength,
		int(main.BPM), main.Approved, time.Now().Unix(), b.frozen,
	)
	if err != nil {
		return err
	}
	return nil
}

func init() {
	osuapi.RateLimit(200)
}
