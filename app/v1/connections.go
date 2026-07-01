package v1

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/osuAkatsuki/akatsuki-api/common"
)

func DiscordUnlinkPOST(md common.MethodData) common.CodeMessager {
	err := md.DB.QueryRow("SELECT 1 FROM users WHERE id = ? AND discord_account_id IS NOT NULL", md.ID()).Scan(new(int))
	switch {
	case err == sql.ErrNoRows:
		var r common.ResponseBase
		r.Code = 400
		r.Message = "You do not have a Discord account linked!"
		return r
	case err != nil:
		md.Err(err)
		return Err500
	}

	md.DB.Exec("UPDATE users SET discord_account_id = NULL WHERE id = ?", md.ID())

	return common.SimpleResponse(200, "Discord unlinked successfully")
}

func DiscordCallbackGET(md common.MethodData) common.CodeMessager {
	code := md.Query("code")
	if code == "" {
		return ErrBadJSON
	}

	if r := validateOAuthState(md, "discord"); r != nil {
		return r
	}

	if md.DB.QueryRow("SELECT 1 FROM users WHERE id = ? AND discord_account_id IS NOT NULL", md.ID()).
		Scan(new(int)) != sql.ErrNoRows {
		var r common.ResponseBase
		r.Code = 403
		r.Message = "You already have a Discord account linked!"
		return r
	}

	settings := common.GetSettings()

	client := resty.New()
	resp, err := client.R().
		SetFormData(map[string]string{
			"client_id":     settings.DISCORD_CLIENT_ID,
			"client_secret": settings.DISCORD_CLIENT_SECRET,
			"grant_type":    "authorization_code",
			"code":          code,
			"redirect_uri":  settings.DISCORD_REDIRECT_URI,
		}).
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		Post("https://discord.com/api/oauth2/token")

	if err != nil {
		md.Err(err)
		return Err500
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}

	if err := json.Unmarshal(resp.Body(), &tokenResp); err != nil {
		md.Err(err)
		return Err500
	}

	userResp, err := client.R().
		SetAuthToken(tokenResp.AccessToken).
		Get("https://discord.com/api/users/@me")

	if err != nil {
		md.Err(err)
		return Err500
	}

	var discordUser struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(userResp.Body(), &discordUser); err != nil {
		md.Err(err)
		return Err500
	}

	md.DB.Exec("UPDATE users SET discord_account_id = ? WHERE id = ?", discordUser.ID, md.ID())

	md.Ctx.Redirect("https://akatsuki.gg/settings/connections", 302)
	return common.SimpleResponse(302, "")
}

func TwitchUnlinkPOST(md common.MethodData) common.CodeMessager {
	err := md.DB.QueryRow("SELECT 1 FROM users WHERE id = ? AND twitch_account_id IS NOT NULL", md.ID()).Scan(new(int))
	switch {
	case err == sql.ErrNoRows:
		var r common.ResponseBase
		r.Code = 400
		r.Message = "You do not have a Twitch account linked!"
		return r
	case err != nil:
		md.Err(err)
		return Err500
	}

	_, err = md.DB.Exec("UPDATE users SET twitch_account_id = NULL, twitch_username = NULL WHERE id = ?", md.ID())
	if err != nil {
		md.Err(err)
		return Err500
	}

	return common.SimpleResponse(200, "Twitch unlinked successfully")
}

func TwitchCallbackGET(md common.MethodData) common.CodeMessager {
	code := md.Query("code")
	if code == "" {
		return ErrBadJSON
	}

	if r := validateOAuthState(md, "twitch"); r != nil {
		return r
	}

	if md.DB.QueryRow("SELECT 1 FROM users WHERE id = ? AND twitch_account_id IS NOT NULL", md.ID()).
		Scan(new(int)) != sql.ErrNoRows {
		var r common.ResponseBase
		r.Code = 403
		r.Message = "You already have a Twitch account linked!"
		return r
	}

	settings := common.GetSettings()
	if settings.TWITCH_CLIENT_ID == "" || settings.TWITCH_CLIENT_SECRET == "" || settings.TWITCH_REDIRECT_URI == "" {
		return common.SimpleResponse(503, "Twitch account linking is not configured.")
	}

	client := resty.New()
	resp, err := client.R().
		SetFormData(map[string]string{
			"client_id":     settings.TWITCH_CLIENT_ID,
			"client_secret": settings.TWITCH_CLIENT_SECRET,
			"grant_type":    "authorization_code",
			"code":          code,
			"redirect_uri":  settings.TWITCH_REDIRECT_URI,
		}).
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		Post("https://id.twitch.tv/oauth2/token")

	if err != nil {
		md.Err(err)
		return Err500
	}

	if resp.IsError() {
		return common.SimpleResponse(resp.StatusCode(), "Failed to exchange Twitch OAuth code.")
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}

	if err := json.Unmarshal(resp.Body(), &tokenResp); err != nil {
		md.Err(err)
		return Err500
	}

	if tokenResp.AccessToken == "" {
		return common.SimpleResponse(502, "Twitch OAuth response did not include an access token.")
	}

	userResp, err := client.R().
		SetAuthToken(tokenResp.AccessToken).
		SetHeader("Client-Id", settings.TWITCH_CLIENT_ID).
		Get("https://api.twitch.tv/helix/users")

	if err != nil {
		md.Err(err)
		return Err500
	}

	if userResp.IsError() {
		return common.SimpleResponse(userResp.StatusCode(), "Failed to fetch Twitch user.")
	}

	var twitchUserResp struct {
		Data []struct {
			ID    string `json:"id"`
			Login string `json:"login"`
		} `json:"data"`
	}

	if err := json.Unmarshal(userResp.Body(), &twitchUserResp); err != nil {
		md.Err(err)
		return Err500
	}

	if len(twitchUserResp.Data) == 0 {
		return common.SimpleResponse(502, "Twitch did not return a user for this OAuth token.")
	}

	twitchUser := twitchUserResp.Data[0]
	var linkedUserID int
	err = md.DB.QueryRow(
		"SELECT id FROM users WHERE twitch_account_id = ? AND id != ? LIMIT 1",
		twitchUser.ID,
		md.ID(),
	).Scan(&linkedUserID)
	switch {
	case err == nil:
		return common.SimpleResponse(409, "That Twitch account is already linked to another Akatsuki account.")
	case err != sql.ErrNoRows:
		md.Err(err)
		return Err500
	}

	_, err = md.DB.Exec(
		"UPDATE users SET twitch_account_id = ?, twitch_username = ? WHERE id = ?",
		twitchUser.ID,
		twitchUser.Login,
		md.ID(),
	)
	if err != nil {
		md.Err(err)
		return Err500
	}

	md.Ctx.Redirect("https://akatsuki.gg/settings/connections", 302)
	return common.SimpleResponse(302, "")
}

func OfficialOsuUnlinkPOST(md common.MethodData) common.CodeMessager {
	err := md.DB.QueryRow("SELECT 1 FROM users WHERE id = ? AND official_osu_user_id IS NOT NULL", md.ID()).Scan(new(int))
	switch {
	case err == sql.ErrNoRows:
		var r common.ResponseBase
		r.Code = 400
		r.Message = "You do not have an official osu! account linked!"
		return r
	case err != nil:
		md.Err(err)
		return Err500
	}

	_, err = md.DB.Exec("UPDATE users SET official_osu_user_id = NULL, official_osu_username = NULL WHERE id = ?", md.ID())
	if err != nil {
		md.Err(err)
		return Err500
	}

	return common.SimpleResponse(200, "Official osu! account unlinked successfully")
}

func OfficialOsuCallbackGET(md common.MethodData) common.CodeMessager {
	code := md.Query("code")
	if code == "" {
		return ErrBadJSON
	}

	if r := validateOAuthState(md, "osu"); r != nil {
		return r
	}

	if md.DB.QueryRow("SELECT 1 FROM users WHERE id = ? AND official_osu_user_id IS NOT NULL", md.ID()).
		Scan(new(int)) != sql.ErrNoRows {
		var r common.ResponseBase
		r.Code = 403
		r.Message = "You already have an official osu! account linked!"
		return r
	}

	settings := common.GetSettings()
	client := resty.New()
	resp, err := client.R().
		SetFormData(map[string]string{
			"client_id":     settings.OSU_OAUTH_CLIENT_ID,
			"client_secret": settings.OSU_OAUTH_CLIENT_SECRET,
			"grant_type":    "authorization_code",
			"code":          code,
			"redirect_uri":  settings.OSU_OAUTH_REDIRECT_URI,
		}).
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		Post("https://osu.ppy.sh/oauth/token")

	if err != nil {
		md.Err(err)
		return Err500
	}

	if resp.IsError() {
		return common.SimpleResponse(resp.StatusCode(), "Failed to exchange osu! OAuth code.")
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}

	if err := json.Unmarshal(resp.Body(), &tokenResp); err != nil {
		md.Err(err)
		return Err500
	}

	if tokenResp.AccessToken == "" {
		return common.SimpleResponse(502, "osu! OAuth response did not include an access token.")
	}

	userResp, err := client.R().
		SetAuthToken(tokenResp.AccessToken).
		SetHeader("Accept", "application/json").
		Get("https://osu.ppy.sh/api/v2/me/osu")

	if err != nil {
		md.Err(err)
		return Err500
	}

	if userResp.IsError() {
		return common.SimpleResponse(userResp.StatusCode(), "Failed to fetch official osu! user.")
	}

	var osuUser struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
	}

	if err := json.Unmarshal(userResp.Body(), &osuUser); err != nil {
		md.Err(err)
		return Err500
	}

	if osuUser.ID <= 0 {
		return common.SimpleResponse(502, "osu! did not return a user for this OAuth token.")
	}

	var linkedUserID int
	err = md.DB.QueryRow(
		"SELECT id FROM users WHERE official_osu_user_id = ? AND id != ? LIMIT 1",
		osuUser.ID,
		md.ID(),
	).Scan(&linkedUserID)
	switch {
	case err == nil:
		return common.SimpleResponse(409, "That official osu! account is already linked to another Akatsuki account.")
	case err != sql.ErrNoRows:
		md.Err(err)
		return Err500
	}

	_, err = md.DB.Exec(
		"UPDATE users SET official_osu_user_id = ?, official_osu_username = ? WHERE id = ?",
		osuUser.ID,
		osuUser.Username,
		md.ID(),
	)
	if err != nil {
		md.Err(err)
		return Err500
	}

	md.Ctx.Redirect("https://akatsuki.gg/settings/connections", 302)
	return common.SimpleResponse(302, "")
}

func validateOAuthState(md common.MethodData, provider string) common.CodeMessager {
	settings := common.GetSettings()
	state := md.Query("state")
	if state == "" {
		return common.SimpleResponse(400, "Missing OAuth state. Please try linking your account again.")
	}

	userID, err := common.ValidateOAuthState(state, provider, settings.HANAYO_KEY, time.Now())
	if err != nil {
		return common.SimpleResponse(400, common.OAuthStateValidationMessage(err))
	}

	if userID != md.ID() {
		return common.SimpleResponse(400, common.OAuthStateValidationMessage(common.ErrInvalidOAuthState))
	}

	return nil
}
