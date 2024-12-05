package common

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

func getEnv(key string) string {
	val, exists := os.LookupEnv(key)
	if !exists {
		panic("Missing environment variable: " + key)
	}
	return val
}

func strToInt(s string) int {
	val, _ := strconv.Atoi(s)
	return val
}

func strToBool(s string) bool {
	val, _ := strconv.ParseBool(s)
	return val
}

type Settings struct {
	APP_PORT   int
	APP_DOMAIN string

	HANAYO_KEY string

	OSU_API_KEY string

	DB_SCHEME string
	DB_HOST   string
	DB_PORT   int
	DB_USER   string
	DB_PASS   string
	DB_NAME   string

	REDIS_HOST            string
	REDIS_PORT            int
	REDIS_PASS            string
	REDIS_DB              int
	REDIS_USE_SSL         bool
	REDIS_SSL_SERVER_NAME string

	DISCORD_CLIENT_ID     string
	DISCORD_CLIENT_SECRET string
	DISCORD_REDIRECT_URI  string
}

var settings = Settings{}

func LoadSettings() Settings {
	godotenv.Load()

	settings.APP_PORT = strToInt(getEnv("APP_PORT"))
	settings.APP_DOMAIN = getEnv("APP_DOMAIN")

	settings.HANAYO_KEY = getEnv("HANAYO_KEY")

	settings.OSU_API_KEY = getEnv("OSU_API_KEY")

	settings.DB_SCHEME = getEnv("DB_SCHEME")
	settings.DB_HOST = getEnv("DB_HOST")
	settings.DB_PORT = strToInt(getEnv("DB_PORT"))
	settings.DB_USER = getEnv("DB_USER")
	settings.DB_PASS = getEnv("DB_PASS")
	settings.DB_NAME = getEnv("DB_NAME")

	settings.REDIS_HOST = getEnv("REDIS_HOST")
	settings.REDIS_PORT = strToInt(getEnv("REDIS_PORT"))
	settings.REDIS_PASS = getEnv("REDIS_PASS")
	settings.REDIS_DB = strToInt(getEnv("REDIS_DB"))
	settings.REDIS_USE_SSL = strToBool(getEnv("REDIS_USE_SSL"))
	settings.REDIS_SSL_SERVER_NAME = getEnv("REDIS_SSL_SERVER_NAME")

	settings.DISCORD_CLIENT_ID = getEnv("DISCORD_CLIENT_ID")
	settings.DISCORD_CLIENT_SECRET = getEnv("DISCORD_CLIENT_SECRET")
	settings.DISCORD_REDIRECT_URI = getEnv("DISCORD_REDIRECT_URI")

	return settings
}

func GetSettings() Settings {
	return settings
}
