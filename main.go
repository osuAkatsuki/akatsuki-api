package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"syscall"

	"golang.org/x/exp/slog"
	"gopkg.in/redis.v5"

	"github.com/osuAkatsuki/akatsuki-api/app"
	"github.com/osuAkatsuki/akatsuki-api/common"
	"github.com/valyala/fasthttp"

	// Golint pls dont break balls
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/serenize/snaker"
)

func init() {
	log.SetFlags(log.Ltime)
	log.SetPrefix(fmt.Sprintf("%d|", syscall.Getpid()))
}

func main() {
	slog.Info("Akatsuki API")

	settings := common.LoadSettings()

	dns := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4,utf8&collation=utf8mb4_general_ci",
		settings.DB_USER,
		settings.DB_PASS,
		settings.DB_HOST,
		settings.DB_PORT,
		settings.DB_NAME,
	)

	dbConn, err := sqlx.Open(settings.DB_SCHEME, dns)
	if err != nil {
		slog.Error("Error opening DB connection", "error", err.Error())
	}

	dbConn.MapperFunc(func(s string) string {
		if x, ok := commonClusterfucks[s]; ok {
			return x
		}
		return snaker.CamelToSnake(s)
	})

	// initialise redis
	var tlsConfig *tls.Config
	if settings.REDIS_USE_SSL {
		tlsConfig = &tls.Config{
			ServerName: settings.REDIS_SSL_SERVER_NAME,
		}
	}
	redisConn := redis.NewClient(&redis.Options{
		Addr:      fmt.Sprintf("%s:%d", settings.REDIS_HOST, settings.REDIS_PORT),
		Password:  settings.REDIS_PASS,
		DB:        settings.REDIS_DB,
		TLSConfig: tlsConfig,
	})

	engine := app.Start(dbConn, redisConn)

	err = fasthttp.ListenAndServe(fmt.Sprintf(":%d", settings.APP_PORT), engine.Handler)
	if err != nil {
		slog.Error("Unable to start server", "error", err.Error())
	}
}

var commonClusterfucks = map[string]string{
	"RegisteredOn": "register_datetime",
	"UsernameAKA":  "username_aka",
	"BeatmapMD5":   "beatmap_md5",
	"Count300":     "300_count",
	"Count100":     "100_count",
	"Count50":      "50_count",
	"CountGeki":    "gekis_count",
	"CountKatu":    "katus_count",
	"CountMiss":    "misses_count",
	"PP":           "pp",
}
