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

type Settings struct {
	APP_PORT int

	HANAYO_KEY                string
	BEATMAP_REQUESTS_PER_USER int
	RANK_QUEUE_SIZE           int

	OSU_API_KEY string

	DB_TYPE string
	DB_HOST string
	DB_PORT int
	DB_USER string
	DB_PASS string
	DB_NAME string

	REDIS_HOST string
	REDIS_PORT int
	REDIS_PASS string
	REDIS_DB   int
}

var settings = Settings{}

func LoadSettings() Settings {
	godotenv.Load()

	settings.APP_PORT = strToInt(getEnv("APP_PORT"))

	settings.HANAYO_KEY = getEnv("HANAYO_KEY")
	settings.BEATMAP_REQUESTS_PER_USER = strToInt(getEnv("BEATMAP_REQUESTS_PER_USER"))
	settings.RANK_QUEUE_SIZE = strToInt(getEnv("RANK_QUEUE_SIZE"))

	settings.OSU_API_KEY = getEnv("OSU_API_KEY")

	settings.DB_TYPE = getEnv("DB_TYPE")
	settings.DB_HOST = getEnv("DB_HOST")
	settings.DB_PORT = strToInt(getEnv("DB_PORT"))
	settings.DB_USER = getEnv("DB_USER")
	settings.DB_PASS = getEnv("DB_PASS")
	settings.DB_NAME = getEnv("DB_NAME")

	settings.REDIS_HOST = getEnv("REDIS_HOST")
	settings.REDIS_PORT = strToInt(getEnv("REDIS_PORT"))
	settings.REDIS_PASS = getEnv("REDIS_PASSWORD")
	settings.REDIS_DB = strToInt(getEnv("REDIS_DB"))

	return settings
}

func GetSettings() Settings {
	return settings
}
