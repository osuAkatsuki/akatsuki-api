// Package peppy implements the osu! API as defined on the osu-api repository wiki (https://github.com/ppy/osu-api/wiki).
package peppy

import (
	"database/sql"
	"fmt"
	"time"

	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/osuAkatsuki/akatsuki-api/common"
	"github.com/thehowl/go-osuapi"
	"github.com/valyala/fasthttp"
	"gopkg.in/redis.v5"
	"zxq.co/ripple/ocl"
)

// R is a redis client.
var R *redis.Client

// GetUser retrieves general user information.
func GetUser(c *fasthttp.RequestCtx, db *sqlx.DB) {
	args := c.QueryArgs()

	if !args.Has("u") {
		json(c, 200, defaultResponse)
		return
	}
	var user osuapi.User
	whereClause, p := genUser(c, db)
	whereClause = "WHERE " + whereClause

	mode := genmode(query(c, "m"))
	rx := query(c, "rx")

	table := "users_stats"
	addJoinClause := ""
	redisTable := "leaderboard"
	switch rx {
	case "1":
		table = "rx_stats"
		addJoinClause = "LEFT JOIN users_stats ON users_stats.id = users.id"
		redisTable = "relaxboard"
	case "2":
		table = "ap_stats"
		addJoinClause = "LEFT JOIN users_stats ON users_stats.id = users.id"
		redisTable = "autoboard"
	}

	var joinDate int64
	err := db.QueryRow(fmt.Sprintf(
		`SELECT
			users.id, users.username, users.register_datetime,
			%[1]s.playcount_%[2]s, %[1]s.ranked_score_%[2]s, %[1]s.total_score_%[2]s,
			%[1]s.pp_%[2]s, %[1]s.avg_accuracy_%[2]s,
			users_stats.country
		FROM users
		LEFT JOIN %[1]s ON %[1]s.id = users.id
		%[3]s
		%[4]s
		LIMIT 1`,
		table, mode, addJoinClause, whereClause,
	), p).Scan(
		&user.UserID, &user.Username, &joinDate,
		&user.Playcount, &user.RankedScore, &user.TotalScore,
		&user.PP, &user.Accuracy,
		&user.Country,
	)
	if err != nil {
		json(c, 200, defaultResponse)
		if err != sql.ErrNoRows {
			common.Err(c, err)
		}
		return
	}

	user.Date = osuapi.MySQLDate(time.Unix(joinDate, 0))

	if gRank := leaderboardPosition(
		R, "ripple:"+redisTable+":"+mode, user.UserID,
	); gRank != nil {
		user.Rank = *gRank
	}

	if cRank := leaderboardPosition(
		R, "ripple:"+redisTable+":"+mode+":"+strings.ToLower(user.Country), user.UserID,
	); cRank != nil {
		user.CountryRank = *cRank
	}

	user.Level = ocl.GetLevelPrecise(user.TotalScore)

	json(c, 200, []osuapi.User{user})
}
