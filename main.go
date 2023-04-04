package main

import (
	"fmt"
	"log"
	"os"
	"syscall"

	"github.com/osuAkatsuki/akatsuki-api/app"
	"github.com/osuAkatsuki/akatsuki-api/beatmapget"
	"github.com/valyala/fasthttp"

	// Golint pls dont break balls
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"github.com/serenize/snaker"
	"gopkg.in/thehowl/go-osuapi.v1"
)

func init() {
	log.SetFlags(log.Ltime)
	log.SetPrefix(fmt.Sprintf("%d|", syscall.Getpid()))
}

var db *sqlx.DB

func main() {

	fmt.Print("Akatsuki API")
	fmt.Println()

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env configuration file: ", err)
	}

	dns := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4,utf8&collation=utf8mb4_general_ci",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASS"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)

	db, err = sqlx.Open(os.Getenv("DB_TYPE"), dns)
	if err != nil {
		log.Fatalln(err)
	}

	db.MapperFunc(func(s string) string {
		if x, ok := commonClusterfucks[s]; ok {
			return x
		}
		return snaker.CamelToSnake(s)
	})

	beatmapget.Client = osuapi.NewClient(os.Getenv("OSU_API_KEY"))
	beatmapget.DB = db

	engine := app.Start(db)

	err = fasthttp.ListenAndServe(":"+os.Getenv("APP_PORT"), engine.Handler)
	if err != nil {
		log.Fatalln(err)
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
