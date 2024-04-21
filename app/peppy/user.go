// Package peppy implements the osu! API as defined on the osu-api repository wiki (https://github.com/ppy/osu-api/wiki).
package peppy

import (
	"database/sql"
	"fmt"
	"strconv"
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
	whereClause = "WHERE " + whereClause + " AND user_stats.mode = ? "

	mode := genmodei(query(c, "m"))
	modeStr := genmode(query(c, "m"))
	rx, err := strconv.Atoi(query(c, "rx"))
	if err != nil {
		json(c, 200, defaultResponse)
		return
	}

	redisTable := "leaderboard"
	switch rx {
	case 1:
		redisTable = "relaxboard"
	case 2:
		redisTable = "autoboard"
	}

	var joinDate int64
	err = db.QueryRow(fmt.Sprintf(
		`SELECT
			users.id, users.username, users.register_datetime, users.country,

			user_stats.playcount, user_stats.ranked_score, user_stats.total_score,
			user_stats.pp, user_stats.avg_accuracy
		FROM users
		LEFT JOIN user_stats ON user_stats.user_id = users.id
		%[1]s
		LIMIT 1`,
		whereClause,
	), p, mode+(4*rx)).Scan(
		&user.UserID, &user.Username, &joinDate, &user.Country,
		&user.Playcount, &user.RankedScore, &user.TotalScore,
		&user.PP, &user.Accuracy,
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
		R, "ripple:"+redisTable+":"+modeStr, user.UserID,
	); gRank != nil {
		user.Rank = *gRank
	}

	if cRank := leaderboardPosition(
		R, "ripple:"+redisTable+":"+modeStr+":"+strings.ToLower(user.Country), user.UserID,
	); cRank != nil {
		user.CountryRank = *cRank
	}

	user.Level = ocl.GetLevelPrecise(user.TotalScore)

	json(c, 200, []osuapi.User{user})
}
