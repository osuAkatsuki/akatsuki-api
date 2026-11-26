package v1

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/osuAkatsuki/akatsuki-api/common"
	"gopkg.in/redis.v5"
	"gopkg.in/thehowl/go-osuapi.v1"
	"zxq.co/ripple/ocl"
	"zxq.co/x/getrank"
)

// What a game client's home screen shows a player, on one kind of
// osu!standard leaderboard (rx: 0 vanilla, 1 relax, 2 autopilot), in one
// answer: their stats and history, their #1s with the player behind each,
// their best scores, the players ranked around them, their plays per day,
// the most played maps this week, the front page's activity, and the news.

const (
	homeHistoryDays = 91
	homeScores      = 50
	homeLadderSize  = 11
	homeNews        = 20
	// Ranked players inactive for this long leave the leaderboards.
	homeActiveDays = 60
	// Plays per day are counted again at most this often.
	homeDaysCacheSeconds = 600
)

// A kind of osu!standard leaderboard: the stats row's mode, its scores'
// table, its leaderboard in redis, and whether its #1 is the most pp
// (relax and autopilot) or the most score (vanilla).
type homeKind struct {
	statsMode int
	table     string
	board     string
	byPP      bool
}

var homeKinds = [...]homeKind{
	{0, "scores", "ripple:leaderboard:std", false},
	{4, "scores_relax", "ripple:relaxboard:std", true},
	{8, "scores_ap", "ripple:autoboard:std", true},
}

// A player at a moment: one of profile-history's daily captures, or now.
type homeCapture struct {
	Time        int64 `json:"time"`
	Rank        int   `json:"rank"`
	CountryRank int   `json:"country_rank"`
	PP          int   `json:"pp"`
}

type homePP struct {
	Time int64 `json:"time"`
	PP   int   `json:"pp"`
}

type homeStats struct {
	PP          int     `json:"pp"`
	Accuracy    float64 `json:"accuracy"`
	PlayCount   int     `json:"playcount"`
	Level       float64 `json:"level"`
	MaxCombo    int     `json:"max_combo"`
	GlobalRank  int     `json:"global_rank"`  // 0 when unranked
	CountryRank int     `json:"country_rank"` // 0 when unranked
	// Whether they gained pp in the last 60 days, which keeps them ranked;
	// and the global rank their pp would have, ranked or not.
	Active    bool `json:"active"`
	RankForPP int  `json:"rank_for_pp"`
}

// The best other score on the map of one of the player's #1s.
type homeSecond struct {
	ScoreID  string  `json:"score_id"`
	UserID   int     `json:"user_id"`
	Username string  `json:"username"`
	Score    int64   `json:"score"`
	PP       float32 `json:"pp"`
	Accuracy float64 `json:"accuracy"`
}

type homeFirstPlace struct {
	userScore
	Second *homeSecond `json:"second"`
}

type homeLadderPlayer struct {
	Rank     int      `json:"rank"`
	UserID   int      `json:"user_id"`
	Username string   `json:"username"`
	Country  string   `json:"country"`
	PP       int      `json:"pp"`
	History  []homePP `json:"history"`
}

type homeDay struct {
	Day        int64   `json:"day"` // days since 1970, at the asked UTC offset
	Plays      int     `json:"plays"`
	Passes     int     `json:"passes"`
	FullCombos int     `json:"full_combos"`
	Accuracy   float64 `json:"accuracy"` // the passes' mean, percent
}

type homeNewsItem struct {
	ID           int     `json:"id"`
	Kind         string  `json:"kind"`
	Title        string  `json:"title"`
	Body         *string `json:"body"`
	URL          *string `json:"url"`
	AuthorID     *int    `json:"author_id"`
	Author       *string `json:"author"`
	BeatmapsetID *int    `json:"beatmapset_id"`
	BeatmapID    *int    `json:"beatmap_id"`
	Time         int64   `json:"time"`
}

type userHomeResponse struct {
	common.ResponseBase
	User struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Country  string `json:"country"`
	} `json:"user"`
	Stats          homeStats     `json:"stats"`
	RankedPlayers  int           `json:"ranked_players"`
	CountryPlayers int           `json:"country_players"`
	History        []homeCapture `json:"history"`
	PeakRank       *homePeak     `json:"peak_rank"`
	FirstPlaces    struct {
		Total  int              `json:"total"`
		Scores []homeFirstPlace `json:"scores"`
	} `json:"first_places"`
	Best     []userScore        `json:"best"`
	Ladder   []homeLadderPlayer `json:"ladder"`
	Days     []homeDay          `json:"days"`
	Trending json.RawMessage    `json:"trending"`
	Activity struct {
		FirstPlaces json.RawMessage `json:"first_places"`
		HighPP      json.RawMessage `json:"high_pp"`
	} `json:"activity"`
	News []homeNewsItem `json:"news"`
}

type homePeak struct {
	Rank int   `json:"rank"`
	Time int64 `json:"time"`
}

// UserHomeGET is /api/v1/users/home?id=&rx=&utc_offset=, utc_offset in
// minutes, which days are counted in.
func UserHomeGET(md common.MethodData) common.CodeMessager {
	userID := common.Int(md.Query("id"))
	rx := common.Int(md.Query("rx"))
	if rx < 0 || rx >= len(homeKinds) {
		return common.SimpleResponse(400, "invalid rx")
	}
	offset := homeUTCOffset(md.Query("utc_offset"))
	kind := homeKinds[rx]
	now := time.Now()

	var r userHomeResponse
	err := md.DB.QueryRow(
		"SELECT users.id, users.username, users.country FROM users "+
			"WHERE users.id = ? AND "+md.User.OnlyUserPublic(true),
		userID,
	).Scan(&r.User.ID, &r.User.Username, &r.User.Country)
	switch {
	case err == sql.ErrNoRows:
		return common.SimpleResponse(404, "That user could not be found!")
	case err != nil:
		md.Err(err)
		return Err500
	}

	steps := []func() error{
		func() error { return homeFillStats(md, &r, kind, now) },
		func() error { return homeFillHistory(md, &r, kind, now) },
		func() error { return homeFillFirstPlaces(md, &r, kind, rx) },
		func() error { return homeFillBest(md, &r, kind) },
		func() error { return homeFillLadder(md, &r, kind, now) },
		func() error { return homeFillDays(md, &r, kind, offset, now) },
		func() error { return homeFillNews(md, &r) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			md.Err(err)
			return Err500
		}
	}
	r.Trending = homeCached(md.R, fmt.Sprintf("akatsuki:trending_beatmaps:%d", kind.statsMode))
	r.Activity.FirstPlaces = homeCached(md.R, fmt.Sprintf("akatsuki:first_places:%d", kind.statsMode))
	r.Activity.HighPP = homeCached(md.R, fmt.Sprintf("akatsuki:high_pp:%d", kind.statsMode))
	r.Code = 200
	return r
}

// The UTC offset asked for, in seconds: whole minutes from -14 to +14 hours.
func homeUTCOffset(minutes string) int64 {
	m, err := strconv.Atoi(minutes)
	if err != nil || m < -14*60 || m > 14*60 {
		return 0
	}
	return int64(m) * 60
}

// The first of `size` players ranked around the one at `index` (from 0) on
// a board of `count`: them in the middle, unless that runs past an end.
func homeLadderStart(index, size, count int64) int64 {
	start := index - size/2
	if start > count-size {
		start = count - size
	}
	if start < 0 {
		start = 0
	}
	return start
}

// A JSON list kept in redis by the cron, or an empty one.
func homeCached(r *redis.Client, key string) json.RawMessage {
	value, err := r.Get(key).Result()
	if err != nil || !json.Valid([]byte(value)) {
		return json.RawMessage("[]")
	}
	return json.RawMessage(value)
}

func homeFillStats(md common.MethodData, r *userHomeResponse, kind homeKind, now time.Time) error {
	var (
		stats       = &r.Stats
		totalScore  int64
		lastAwarded int64
	)
	err := md.DB.QueryRow(`SELECT pp, avg_accuracy, playcount, total_score,
		max_combo, latest_pp_awarded FROM user_stats WHERE user_id = ? AND mode = ?`,
		r.User.ID, kind.statsMode,
	).Scan(&stats.PP, &stats.Accuracy, &stats.PlayCount, &totalScore,
		&stats.MaxCombo, &lastAwarded)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	stats.Level = ocl.GetLevelPrecise(totalScore)
	stats.Active = now.Unix()-lastAwarded < homeActiveDays*86400
	countryBoard := kind.board + ":" + strings.ToLower(r.User.Country)
	if i := _position(md.R, kind.board, r.User.ID); i != nil {
		stats.GlobalRank = *i
	}
	if i := _position(md.R, countryBoard, r.User.ID); i != nil {
		stats.CountryRank = *i
	}
	rankForPP, err := _rankAtPerformancePoints(md.R, kind.board, stats.PP)
	if err != nil {
		return err
	}
	stats.RankForPP = rankForPP
	if r.RankedPlayers, err = homeBoardSize(md.R, kind.board); err != nil {
		return err
	}
	r.CountryPlayers, err = homeBoardSize(md.R, countryBoard)
	return err
}

func homeBoardSize(r *redis.Client, key string) (int, error) {
	size, err := r.ZCard(key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	return int(size), err
}

func homeFillHistory(md common.MethodData, r *userHomeResponse, kind homeKind, now time.Time) error {
	// rank is a reserved word in MySQL 8.
	rows, err := md.DB.Query("SELECT captured_at, `rank`, country_rank, pp "+
		"FROM user_profile_history WHERE user_id = ? AND mode = ? AND captured_at > ? "+
		"ORDER BY captured_at",
		r.User.ID, kind.statsMode, now.Add(-homeHistoryDays*24*time.Hour).UTC())
	if err != nil {
		return err
	}
	defer rows.Close()
	r.History = []homeCapture{}
	for rows.Next() {
		var (
			at      time.Time
			capture homeCapture
			country sql.NullInt64
		)
		if err := rows.Scan(&at, &capture.Rank, &country, &capture.PP); err != nil {
			return err
		}
		capture.Time = at.Unix()
		capture.CountryRank = int(country.Int64)
		r.History = append(r.History, capture)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	r.History = append(r.History, homeCapture{
		Time:        now.Unix(),
		Rank:        r.Stats.GlobalRank,
		CountryRank: r.Stats.CountryRank,
		PP:          r.Stats.PP,
	})

	var (
		peak homePeak
		at   time.Time
	)
	err = md.DB.QueryRow("SELECT `rank`, captured_at FROM user_profile_history "+
		"WHERE user_id = ? AND mode = ? AND `rank` > 0 ORDER BY `rank`, captured_at LIMIT 1",
		r.User.ID, kind.statsMode).Scan(&peak.Rank, &at)
	switch {
	case err == sql.ErrNoRows:
	case err != nil:
		return err
	default:
		peak.Time = at.Unix()
		r.PeakRank = &peak
	}
	if rank := r.Stats.GlobalRank; rank > 0 && (r.PeakRank == nil || rank < r.PeakRank.Rank) {
		r.PeakRank = &homePeak{Rank: rank, Time: now.Unix()}
	}
	return nil
}

// The columns scanUserScore reads, of `table` and beatmaps.
func homeScoreColumns(table string) string {
	return strings.ReplaceAll(`{t}.id, {t}.beatmap_md5, {t}.score,
		{t}.max_combo, {t}.full_combo, {t}.mods,
		{t}.300_count, {t}.100_count, {t}.50_count,
		{t}.gekis_count, {t}.katus_count, {t}.misses_count,
		{t}.time, {t}.play_mode, {t}.accuracy, {t}.pp,
		{t}.completed, {t}.pinned, {t}.userid,
		beatmaps.beatmap_id, beatmaps.beatmapset_id, beatmaps.beatmap_md5,
		beatmaps.song_name, beatmaps.ar, beatmaps.od,
		beatmaps.max_combo, beatmaps.hit_length, beatmaps.ranked,
		beatmaps.ranked_status_freezed, beatmaps.latest_update`, "{t}", table)
}

func scanUserScore(rows *sql.Rows) (userScore, error) {
	var us userScore
	err := rows.Scan(
		&us.ID, &us.BeatmapMD5, &us.Score.Score,
		&us.MaxCombo, &us.FullCombo, &us.Mods,
		&us.Count300, &us.Count100, &us.Count50,
		&us.CountGeki, &us.CountKatu, &us.CountMiss,
		&us.Time, &us.PlayMode, &us.Accuracy, &us.PP,
		&us.Completed, &us.Pinned, &us.UserID,
		&us.Beatmap.BeatmapID, &us.Beatmap.BeatmapsetID, &us.Beatmap.BeatmapMD5,
		&us.Beatmap.SongName, &us.Beatmap.AR, &us.Beatmap.OD,
		&us.Beatmap.MaxCombo, &us.Beatmap.HitLength, &us.Beatmap.Ranked,
		&us.Beatmap.RankedStatusFrozen, &us.Beatmap.LatestUpdate,
	)
	us.Rank = strings.ToUpper(getrank.GetRank(
		osuapi.Mode(us.PlayMode), osuapi.Mods(us.Mods), us.Accuracy,
		us.Count300, us.Count100, us.Count50, us.CountMiss,
	))
	return us, err
}

func homeFillFirstPlaces(md common.MethodData, r *userHomeResponse, kind homeKind, rx int) error {
	err := md.DB.QueryRow(`SELECT COUNT(*) FROM scores_first
		WHERE userid = ? AND mode = 0 AND rx = ?`, r.User.ID, rx).Scan(&r.FirstPlaces.Total)
	if err != nil {
		return err
	}
	rows, err := md.DB.Query(fmt.Sprintf(`SELECT %[2]s FROM scores_first
		INNER JOIN beatmaps ON beatmaps.beatmap_md5 = scores_first.beatmap_md5
		INNER JOIN %[1]s ON %[1]s.id = scores_first.scoreid
		WHERE scores_first.userid = ? AND scores_first.mode = 0 AND scores_first.rx = ?
		ORDER BY %[1]s.time DESC LIMIT %[3]d`, kind.table, homeScoreColumns(kind.table), homeScores),
		r.User.ID, rx)
	if err != nil {
		return err
	}
	defer rows.Close()
	r.FirstPlaces.Scores = []homeFirstPlace{}
	var md5s []string
	for rows.Next() {
		us, err := scanUserScore(rows)
		if err != nil {
			return err
		}
		r.FirstPlaces.Scores = append(r.FirstPlaces.Scores, homeFirstPlace{userScore: us})
		md5s = append(md5s, us.BeatmapMD5)
	}
	if err := rows.Err(); err != nil || len(md5s) == 0 {
		return err
	}

	// The best score on each of those maps by another public player.
	order := "score"
	if kind.byPP {
		order = "pp"
	}
	query, args, err := sqlx.In(fmt.Sprintf(`SELECT beatmap_md5, id, userid, username,
		score, pp, accuracy FROM (
			SELECT s.beatmap_md5, s.id, s.userid, users.username, s.score, s.pp, s.accuracy,
				ROW_NUMBER() OVER (PARTITION BY s.beatmap_md5 ORDER BY s.%[2]s DESC, s.id) AS place
			FROM %[1]s s INNER JOIN users ON users.id = s.userid
			WHERE s.beatmap_md5 IN (?) AND s.play_mode = 0 AND s.completed = 3
				AND s.userid != ? AND users.privileges & 1 = 1
		) others WHERE place = 1`, kind.table, order), md5s, r.User.ID)
	if err != nil {
		return err
	}
	seconds, err := md.DB.Query(md.DB.Rebind(query), args...)
	if err != nil {
		return err
	}
	defer seconds.Close()
	byMap := map[string]*homeSecond{}
	for seconds.Next() {
		var (
			md5    string
			second homeSecond
		)
		if err := seconds.Scan(&md5, &second.ScoreID, &second.UserID, &second.Username,
			&second.Score, &second.PP, &second.Accuracy); err != nil {
			return err
		}
		byMap[md5] = &second
	}
	for i := range r.FirstPlaces.Scores {
		r.FirstPlaces.Scores[i].Second = byMap[r.FirstPlaces.Scores[i].BeatmapMD5]
	}
	return seconds.Err()
}

func homeFillBest(md common.MethodData, r *userHomeResponse, kind homeKind) error {
	rows, err := md.DB.Query(fmt.Sprintf(`SELECT %[2]s FROM %[1]s
		INNER JOIN beatmaps ON beatmaps.beatmap_md5 = %[1]s.beatmap_md5
		WHERE %[1]s.userid = ? AND %[1]s.play_mode = 0 AND %[1]s.completed = 3
			AND beatmaps.ranked IN (2, 3)
		ORDER BY %[1]s.pp DESC, %[1]s.score DESC LIMIT %[3]d`,
		kind.table, homeScoreColumns(kind.table), homeScores), r.User.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	r.Best = []userScore{}
	for rows.Next() {
		us, err := scanUserScore(rows)
		if err != nil {
			return err
		}
		r.Best = append(r.Best, us)
	}
	return rows.Err()
}

// The players ranked around the player, them too, or, unranked, around
// where their pp would rank; each with their pp over the history's days.
func homeFillLadder(md common.MethodData, r *userHomeResponse, kind homeKind, now time.Time) error {
	r.Ladder = []homeLadderPlayer{}
	if r.Stats.GlobalRank == 0 && r.Stats.PP == 0 {
		return nil // never ranked: nowhere to be
	}
	count, err := md.R.ZCard(kind.board).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	index := int64(r.Stats.RankForPP - 1)
	if r.Stats.GlobalRank > 0 {
		index = int64(r.Stats.GlobalRank - 1)
	}
	start := homeLadderStart(index, homeLadderSize, count)
	players, err := md.R.ZRevRangeWithScores(kind.board, start, start+homeLadderSize-1).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	if len(players) == 0 {
		return nil
	}
	var ids []int
	for i, player := range players {
		id, _ := strconv.Atoi(fmt.Sprint(player.Member))
		ids = append(ids, id)
		r.Ladder = append(r.Ladder, homeLadderPlayer{
			Rank: int(start) + i + 1, UserID: id, PP: int(player.Score), History: []homePP{},
		})
	}

	query, args, err := sqlx.In("SELECT id, username, country FROM users WHERE id IN (?)", ids)
	if err != nil {
		return err
	}
	rows, err := md.DB.Query(md.DB.Rebind(query), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	byID := map[int]*homeLadderPlayer{}
	for i := range r.Ladder {
		byID[r.Ladder[i].UserID] = &r.Ladder[i]
	}
	for rows.Next() {
		var (
			id                int
			username, country string
		)
		if err := rows.Scan(&id, &username, &country); err != nil {
			return err
		}
		if player := byID[id]; player != nil {
			player.Username, player.Country = username, country
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	query, args, err = sqlx.In(`SELECT user_id, captured_at, pp FROM user_profile_history
		WHERE user_id IN (?) AND mode = ? AND captured_at > ? ORDER BY captured_at`,
		ids, kind.statsMode, now.Add(-homeHistoryDays*24*time.Hour).UTC())
	if err != nil {
		return err
	}
	history, err := md.DB.Query(md.DB.Rebind(query), args...)
	if err != nil {
		return err
	}
	defer history.Close()
	for history.Next() {
		var (
			id int
			at time.Time
			pp int
		)
		if err := history.Scan(&id, &at, &pp); err != nil {
			return err
		}
		if player := byID[id]; player != nil {
			player.History = append(player.History, homePP{Time: at.Unix(), PP: pp})
		}
	}
	for i := range r.Ladder {
		r.Ladder[i].History = append(r.Ladder[i].History, homePP{Time: now.Unix(), PP: r.Ladder[i].PP})
	}
	return history.Err()
}

// Plays per day over the history's days, kept in redis for a while: it
// reads all of the player's scores of the kind.
func homeFillDays(md common.MethodData, r *userHomeResponse, kind homeKind, offset int64, now time.Time) error {
	key := fmt.Sprintf("akatsuki:home_days:%d:%d:%d", r.User.ID, kind.statsMode, offset)
	if cached, err := md.R.Get(key).Result(); err == nil &&
		json.Unmarshal([]byte(cached), &r.Days) == nil {
		return nil
	}
	rows, err := md.DB.Query(fmt.Sprintf(`SELECT FLOOR((time + ?) / 86400) AS day,
		COUNT(*), SUM(completed >= 2), SUM(completed >= 2 AND full_combo = 1),
		SUM(IF(completed >= 2, accuracy, 0))
		FROM %s WHERE userid = ? AND play_mode = 0 AND time > ?
		GROUP BY day ORDER BY day`, kind.table),
		offset, r.User.ID, now.Unix()-homeHistoryDays*86400)
	if err != nil {
		return err
	}
	defer rows.Close()
	r.Days = []homeDay{}
	for rows.Next() {
		var (
			day         homeDay
			accuracySum float64
		)
		if err := rows.Scan(&day.Day, &day.Plays, &day.Passes, &day.FullCombos, &accuracySum); err != nil {
			return err
		}
		if day.Passes > 0 {
			day.Accuracy = accuracySum / float64(day.Passes)
		}
		r.Days = append(r.Days, day)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if encoded, err := json.Marshal(r.Days); err == nil {
		md.R.Set(key, encoded, homeDaysCacheSeconds*time.Second)
	}
	return nil
}

func homeFillNews(md common.MethodData, r *userHomeResponse) error {
	rows, err := md.DB.Query(fmt.Sprintf(`SELECT news.id, news.kind, news.title,
		news.body, news.url, news.author_id, users.username, news.beatmapset_id,
		news.beatmap_id, news.created_at
		FROM news LEFT JOIN users ON users.id = news.author_id
		ORDER BY news.created_at DESC, news.id DESC LIMIT %d`, homeNews))
	if err != nil {
		return err
	}
	defer rows.Close()
	r.News = []homeNewsItem{}
	for rows.Next() {
		var (
			item homeNewsItem
			at   time.Time
		)
		if err := rows.Scan(&item.ID, &item.Kind, &item.Title, &item.Body, &item.URL,
			&item.AuthorID, &item.Author, &item.BeatmapsetID, &item.BeatmapID, &at); err != nil {
			return err
		}
		item.Time = at.Unix()
		r.News = append(r.News, item)
	}
	return rows.Err()
}
