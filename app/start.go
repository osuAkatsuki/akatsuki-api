package app

import (
	"crypto/tls"
	"fmt"
	"time"

	fhr "github.com/buaazp/fasthttprouter"
	"github.com/jmoiron/sqlx"
	"github.com/osuAkatsuki/akatsuki-api/app/internals"
	"github.com/osuAkatsuki/akatsuki-api/app/peppy"
	v1 "github.com/osuAkatsuki/akatsuki-api/app/v1"
	"github.com/osuAkatsuki/akatsuki-api/app/websockets"
	"github.com/osuAkatsuki/akatsuki-api/common"
	"gopkg.in/redis.v5"
)

var (
	db  *sqlx.DB
	red *redis.Client
)

// Start begins taking HTTP connections.
func Start(dbO *sqlx.DB) *fhr.Router {
	db = dbO

	rawRouter := fhr.New()
	r := router{rawRouter}

	settings := common.GetSettings()

	// initialise redis
	var tlsConfig *tls.Config
	if settings.REDIS_USE_SSL {
		tlsConfig = &tls.Config{
			ServerName: settings.REDIS_SSL_SERVER_NAME,
		}
	}
	red = redis.NewClient(&redis.Options{
		Addr:      fmt.Sprintf("%s:%d", settings.REDIS_HOST, settings.REDIS_PORT),
		Password:  settings.REDIS_PASS,
		DB:        settings.REDIS_DB,
		TLSConfig: tlsConfig,
	})
	peppy.R = red

	// token updater
	go tokenUpdater(db)

	// start websocket
	// websockets.Start(red, db)

	// start load achievements
	go v1.LoadAchievementsEvery(db, time.Minute*10)

	// peppyapi
	{
		r.Peppy("/api/get_user", peppy.GetUser)
		r.Peppy("/api/get_match", peppy.GetMatch)
		r.Peppy("/api/get_user_recent", peppy.GetUserRecent)
		r.Peppy("/api/get_user_best", peppy.GetUserBest)
		r.Peppy("/api/get_scores", peppy.GetScores)
		r.Peppy("/api/get_beatmaps", peppy.GetBeatmap)
	}

	// v1 API
	{
		r.Method("/_health", v1.HealthGET)

		r.POSTMethod("/api/v1/tokens/self/delete", v1.TokenSelfDeletePOST)

		// Auth-free API endpoints (public data)
		r.Method("/api/v1/ping", v1.PingGET)
		r.Method("/api/v1/surprise_me", v1.SurpriseMeGET)

		r.Method("/api/v1/match", v1.MatchGET)

		r.Method("/api/v1/users", v1.UsersGET)
		r.Method("/api/v1/users/whatid", v1.UserWhatsTheIDGET)
		r.Method("/api/v1/users/full", v1.UserFullGET)
		r.Method("/api/v1/users/achievements", v1.UserAchievementsGET)
		r.Method("/api/v1/users/userpage", v1.UserUserpageGET)
		r.Method("/api/v1/users/lookup", v1.UserLookupGET)
		r.Method("/api/v1/users/scores/best", v1.UserScoresBestGET)
		r.Method("/api/v1/users/scores/recent", v1.UserScoresRecentGET)
		r.Method("/api/v1/users/scores/first", v1.UserFirstGET)
		r.Method("/api/v1/users/scores/pinned", v1.UserScoresPinnedGET)
		r.Method("/api/v1/users/most_played", v1.UserMostPlayedBeatmapsGET)
		r.Method("/api/v1/badges", v1.BadgesGET)
		r.Method("/api/v1/badges/members", v1.BadgeMembersGET)
		r.Method("/api/v1/clans", v1.ClansGET)
		r.Method("/api/v1/clans/members", v1.ClanMembersGET)
		r.Method("/api/v1/clans/stats", v1.ClanStatsGET)
		r.Method("/api/v1/clans/stats/all", v1.ClanLeaderboardGET)
		r.Method("/api/v1/clans/stats/first", v1.ClansFirstPlaceRankingGET)
		r.Method("/api/v1/clans/invite", v1.ResolveInviteGET)
		r.Method("/api/v1/tbadges", v1.TBadgesGET)
		r.Method("/api/v1/tbadges/members", v1.TBadgeMembersGET)
		r.Method("/api/v1/beatmaps", v1.BeatmapGET)
		r.Method("/api/v1/leaderboard", v1.LeaderboardGET)
		r.Method("/api/v1/scoreleaderboard", v1.SLeaderboardGET)
		r.Method("/api/v1/tokens", v1.TokenGET)
		r.Method("/api/v1/users/self", v1.UserSelfGET)
		r.Method("/api/v1/tokens/self", v1.TokenSelfGET)
		r.Method("/api/v1/blog/posts", v1.BlogPostsGET)
		r.Method("/api/v1/score", v1.ScoreGET)
		r.Method("/api/v1/scores", v1.ScoresGET)
		r.Method("/api/v1/beatmaps/rank_requests/status", v1.BeatmapRankRequestsStatusGET)
		r.Method("/api/v1/countries", v1.CountriesGET)
		r.Method("/api/v1/hypothetical-rank", v1.HypotheticalRankGET)

		// ReadConfidential privilege required
		r.Method("/api/v1/friends", v1.FriendsGET, common.PrivilegeReadConfidential)
		r.Method("/api/v1/friends/with", v1.FriendsWithGET, common.PrivilegeReadConfidential)
		r.Method("/api/v1/users/self/donor_info", v1.UsersSelfDonorInfoGET, common.PrivilegeReadConfidential)
		r.Method("/api/v1/users/self/favourite_mode", v1.UsersSelfFavouriteModeGET, common.PrivilegeReadConfidential)
		r.Method("/api/v1/users/self/settings", v1.UsersSelfSettingsGET, common.PrivilegeReadConfidential)

		// Write privilege required
		r.POSTMethod("/api/v1/friends/add", v1.FriendsAddPOST, common.PrivilegeWrite)
		r.POSTMethod("/api/v1/friends/del", v1.FriendsDelPOST, common.PrivilegeWrite)
		r.POSTMethod("/api/v1/users/scores/pin", v1.ScoresPinAddPOST, common.PrivilegeWrite)
		r.POSTMethod("/api/v1/users/scores/unpin", v1.ScoresPinDelPOST, common.PrivilegeWrite)
		r.POSTMethod("/api/v1/users/self/settings", v1.UsersSelfSettingsPOST, common.PrivilegeWrite)
		r.POSTMethod("/api/v1/users/self/userpage", v1.UserSelfUserpagePOST, common.PrivilegeWrite)
		r.POSTMethod("/api/v1/beatmaps/rank_requests", v1.BeatmapRankRequestsSubmitPOST, common.PrivilegeWrite)
		r.POSTMethod("/api/v1/clans/join", v1.ClanJoinPOST, common.PrivilegeWrite)
		r.POSTMethod("/api/v1/clans/invite", v1.ClanGenerateInvitePOST, common.PrivilegeWrite)
		r.POSTMethod("/api/v1/clans/leave", v1.ClanLeavePOST, common.PrivilegeWrite)
		r.POSTMethod("/api/v1/clans/settings", v1.ClanSettingsPOST, common.PrivilegeWrite)
		r.POSTMethod("/api/v1/clans/kick", v1.ClanKickPOST, common.PrivilegeWrite)

		// Admin: RAP
		r.POSTMethod("/api/v1/rap/log", v1.RAPLogPOST)

		// Admin: beatmap
		r.POSTMethod("/api/v1/beatmaps/set_status", v1.BeatmapSetStatusPOST, common.PrivilegeBeatmap)
		r.Method("/api/v1/beatmaps/ranked_frozen_full", v1.BeatmapRankedFrozenFullGET, common.PrivilegeBeatmap)

		// Admin: user managing
		r.POSTMethod("/api/v1/users/manage/set_allowed", v1.UserManageSetAllowedPOST, common.PrivilegeManageUser)
		r.POSTMethod("/api/v1/users/edit", v1.UserEditPOST, common.PrivilegeManageUser)
		r.POSTMethod("/api/v1/users/wipe", v1.WipeUserPOST, common.PrivilegeManageUser)
		r.POSTMethod("/api/v1/scores/reports", v1.ScoreReportPOST, common.PrivilegeManageUser)
		r.Method("/api/v1/users/unweighted", v1.UserUnweightedPerformanceGET, common.PrivilegeManageUser)

		// M E T A
		// E     T    "wow thats so meta"
		// T     E                  -- the one who said "wow thats so meta"
		// A T E M
		r.Method("/api/v1/meta/restart", v1.MetaRestartGET, common.PrivilegeAPIMeta)
		r.Method("/api/v1/meta/up_since", v1.MetaUpSinceGET, common.PrivilegeAPIMeta)
		r.Method("/api/v1/meta/update", v1.MetaUpdateGET, common.PrivilegeAPIMeta)
	}

	// Websocket API
	{
		r.PlainGET("/api/v1/ws", websockets.WebsocketV1Entry)
	}

	// in the new osu-web, the old endpoints are also in /v1 it seems. So /shrug
	{
		r.Peppy("/api/v1/get_user", peppy.GetUser)
		r.Peppy("/api/v1/get_match", peppy.GetMatch)
		r.Peppy("/api/v1/get_user_recent", peppy.GetUserRecent)
		r.Peppy("/api/v1/get_user_best", peppy.GetUserBest)
		r.Peppy("/api/v1/get_scores", peppy.GetScores)
		r.Peppy("/api/v1/get_beatmaps", peppy.GetBeatmap)
	}

	r.GET("/api/status", internals.Status)

	rawRouter.NotFound = v1.Handle404

	return rawRouter
}
