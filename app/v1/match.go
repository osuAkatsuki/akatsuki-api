package v1

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/osuAkatsuki/akatsuki-api/common"
)

var scoringTypeMap = map[int]string{
	0: "score",
	1: "accuracy",
	2: "max_combo",
	3: "score",
}

type JsonList []int

func (jl *JsonList) Scan(val interface{}) error {
	switch v := val.(type) {
	case []byte:
		json.Unmarshal(v, &jl)
		return nil
	case string:
		json.Unmarshal([]byte(v), &jl)
		return nil
	default:
		return fmt.Errorf("unsupported type: %T", v)
	}
}
func (jl *JsonList) Value() (driver.Value, error) {
	return json.Marshal(jl)
}

type GameBeatmap struct {
	Id           int    `json:"id"`
	BeatmapsetId int    `json:"beatmapset_id"`
	Artist       string `json:"artist"`
	Title        string `json:"title"`
	Version      string `json:"version"`
}

type MatchUser struct {
	Id       int     `json:"id"`
	Username string  `json:"username"`
	Country  *string `json:"country"`
}

type MatchScore struct {
	Id        int       `json:"id"`
	User      MatchUser `json:"user"`
	Score     int64     `json:"score"`
	MaxCombo  int       `json:"max_combo"`
	Mods      int       `json:"mods"`
	Mode      int       `json:"mode"`
	Count300  int       `json:"count_300"`
	Count100  int       `json:"count_100"`
	Count50   int       `json:"count_50"`
	CountGeki int       `json:"count_geki"`
	CountKatu int       `json:"count_katu"`
	CountMiss int       `json:"count_miss"`
	Timestamp time.Time `json:"timestamp"`
	Accuracy  float64   `json:"accuracy"`
	Passed    bool      `json:"passed"`
	Team      int       `json:"team"`
}

type MatchGame struct {
	Id          int          `json:"id"`
	Beatmap     GameBeatmap  `json:"beatmap"`
	Mode        int          `json:"mode"`
	Mods        *int         `json:"mods"`
	ScoringType int          `json:"scoring_type"`
	TeamType    int          `json:"team_type"`
	StartTime   time.Time    `json:"start_time"`
	EndTime     *time.Time   `json:"end_time"`
	Scores      []MatchScore `json:"scores"`
}

type MatchEvent struct {
	Id        int        `json:"id"`
	Type      string     `json:"type"`
	Timestamp time.Time  `json:"timestamp"`
	User      *MatchUser `json:"user"`
	Game      *MatchGame `json:"game"`
}

type Match struct {
	Id        int        `json:"id"`
	Name      string     `json:"name"`
	StartTime time.Time  `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
}

type matchDataResponse struct {
	common.ResponseBase
	Match         Match        `json:"match"`
	Events        []MatchEvent `json:"events"`
	FirstEventId  int          `json:"first_event_id"`
	LatestEventId int          `json:"latest_event_id"`
	CurrentGameId *int         `json:"current_game_id"`
}

var SongNameRegex = regexp.MustCompile(`^(.+) - (.+) \[(.+)\]$`)

func splitSongName(songName string) (string, string, string) {
	var artist, title, version string
	matches := SongNameRegex.FindStringSubmatch(songName)
	if len(matches) == 4 {
		artist = matches[1]
		title = matches[2]
		version = matches[3]
	}
	return artist, title, version
}

func inList(list []int, value int) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func MatchGET(md common.MethodData) common.CodeMessager {

	r := matchDataResponse{}
	var (
		privateMatch    bool
		participantsIds JsonList
	)

	matchId := common.Int(md.Query("id"))
	beforeEventId := common.Int(md.Query("before"))
	afterEventId := common.Int(md.Query("after"))
	limit := clamp(common.Int(md.Query("limit")), 1, 101)

	err := md.DB.QueryRow(
		"SELECT id, name, private, start_time, end_time FROM matches WHERE id = ? LIMIT 1",
		matchId,
	).Scan(&r.Match.Id, &r.Match.Name, &privateMatch, &r.Match.StartTime, &r.Match.EndTime)

	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "That match could not be found!")
	case err != nil:
		md.Err(err)
		return Err500
	}

	err = md.DB.Get(
		&participantsIds,
		`SELECT JSON_ARRAYAGG(user_id) FROM (
			SELECT DISTINCT(user_id) AS user_id FROM
			match_events WHERE match_id = ? AND event_type = ?
		) AS events`,
		r.Match.Id,
		common.MATCH_USER_JOIN,
	)

	if err != nil {
		md.Err(err)
		return Err500
	}

	if privateMatch &&
		(!inList(participantsIds, md.User.ID) ||
			md.User.UserPrivileges&common.UserPrivilegeTournamentStaff == 0) {
		return common.SimpleResponse(404, "That match could not be found!")
	}

	var extraQuery string
	var args []interface{}

	sortOrderEvents := "DESC"
	requireReverse := true

	args = append(args, r.Match.Id)

	if afterEventId != 0 {
		extraQuery = "AND id > ?"
		args = append(args, afterEventId)

		sortOrderEvents = "ASC"
		requireReverse = false
	} else if beforeEventId != 0 {
		extraQuery = "AND id < ?"
		args = append(args, beforeEventId)
	}

	args = append(args, limit)

	rows, err := md.DB.Query(
		fmt.Sprintf(
			`SELECT id, game_id, user_id, event_type, timestamp
			FROM match_events WHERE match_id = ? %s ORDER BY id %s LIMIT ?`,
			extraQuery, sortOrderEvents,
		),
		args...,
	)

	if err != nil {
		md.Err(err)
		return Err500
	}

	for rows.Next() {
		me := MatchEvent{}

		var userId *int
		var gameId *int

		err = rows.Scan(&me.Id, &gameId, &userId, &me.Type, &me.Timestamp)

		if err != nil {
			md.Err(err)
			continue
		}

		if userId != nil {
			me.User = &MatchUser{}
			err = md.DB.QueryRow(
				"SELECT id, username FROM users WHERE id = ? LIMIT 1",
				userId,
			).Scan(&me.User.Id, &me.User.Username)

			if err != nil {
				md.Err(err)
				continue
			}
		}

		if gameId != nil {
			me.Game = &MatchGame{}
			var songName string
			err = md.DB.QueryRow(
				`SELECT g.id, g.mode, g.mods, g.scoring_type, g.team_type,
				g.start_time, g.end_time, b.beatmap_id, b.beatmapset_id, b.song_name
				FROM match_games g
				INNER JOIN beatmaps b
				ON g.beatmap_id = b.beatmap_id
				WHERE id = ? LIMIT 1`,
				gameId,
			).Scan(
				&me.Game.Id, &me.Game.Mode, &me.Game.Mods, &me.Game.ScoringType,
				&me.Game.TeamType, &me.Game.StartTime, &me.Game.EndTime,
				&me.Game.Beatmap.Id, &me.Game.Beatmap.BeatmapsetId, &songName,
			)

			if err != nil {
				md.Err(err)
				continue
			}

			me.Game.Beatmap.Artist, me.Game.Beatmap.Title, me.Game.Beatmap.Version = splitSongName(songName)

			useAvg := me.Game.ScoringType == 1 || me.Game.ScoringType == 2

			mysqlFunc := "SUM"
			if useAvg {
				mysqlFunc = "AVG"
			}

			var winningTeam int
			err = md.DB.Get(&winningTeam, fmt.Sprintf(
				`SELECT team FROM match_game_scores WHERE game_id = ?
					GROUP BY team ORDER BY %s(%s) DESC LIMIT 1`,
				mysqlFunc, scoringTypeMap[me.Game.ScoringType]),
				me.Game.Id,
			)

			if err != nil && err != sql.ErrNoRows {
				md.Err(err)
				continue
			}

			sortOrder := "ASC"
			if winningTeam == 2 {
				sortOrder = "DESC"
			}

			scoreRows, err := md.DB.Query(fmt.Sprintf(
				`SELECT u.id, u.username, u.country, s.id, s.count_300, s.count_100, s.count_50,
					s.count_geki, s.count_katu, s.count_miss, s.score, s.accuracy, s.max_combo,
					s.mods, s.mode, s.passed, s.team, s.timestamp
					FROM match_game_scores s
					INNER JOIN users u ON s.user_id = u.id
					WHERE match_id = ? AND game_id = ? ORDER BY team %s, passed DESC, %s DESC`,
				sortOrder, scoringTypeMap[me.Game.ScoringType],
			),
				r.Match.Id, me.Game.Id,
			)

			if err != nil {
				md.Err(err)
				continue
			}

			for scoreRows.Next() {
				ms := MatchScore{}

				err = scoreRows.Scan(
					&ms.User.Id, &ms.User.Username, &ms.User.Country, &ms.Id, &ms.Count300, &ms.Count100,
					&ms.Count50, &ms.CountGeki, &ms.CountKatu, &ms.CountMiss, &ms.Score,
					&ms.Accuracy, &ms.MaxCombo, &ms.Mods, &ms.Mode, &ms.Passed, &ms.Team, &ms.Timestamp,
				)

				if err != nil {
					md.Err(err)
					continue
				}

				me.Game.Scores = append(me.Game.Scores, ms)
			}
		}

		r.Events = append(r.Events, me)
	}

	// This is so dumb
	if requireReverse {
		for i, j := 0, len(r.Events)-1; i < j; i, j = i+1, j-1 {
			r.Events[i], r.Events[j] = r.Events[j], r.Events[i]
		}
	}

	err = md.DB.QueryRow(
		`SELECT MIN(id) first_event_id, MAX(id) latest_event_id FROM match_events WHERE match_id = ?`,
		r.Match.Id,
	).Scan(&r.FirstEventId, &r.LatestEventId)

	if err != nil {
		md.Err(err)
		return Err500
	}

	err = md.DB.Get(
		&r.CurrentGameId,
		`SELECT id FROM match_games WHERE match_id = ? AND end_time IS NULL ORDER BY id DESC LIMIT 1`,
		r.Match.Id,
	)

	if err != nil && err != sql.ErrNoRows {
		md.Err(err)
		return Err500
	}

	r.ResponseBase.Code = 200
	return r
}
