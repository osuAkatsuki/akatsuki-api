package v1

import (
	"encoding/json"

	"github.com/go-resty/resty/v2"
	"github.com/osuAkatsuki/akatsuki-api/common"
)

func DiscordCallbackGET(md common.MethodData) common.CodeMessager {
	state := md.Query("state")
	if state == "" {
		return ErrBadJSON
	}

	code := md.Query("code")
	if code == "" {
		return ErrBadJSON
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
		ID int64 `json:"id"`
	}

	if err := json.Unmarshal(userResp.Body(), &discordUser); err != nil {
		md.Err(err)
		return Err500
	}

	md.DB.Exec("UPDATE users SET discord_account_id = ? WHERE id = ?", discordUser.ID, md.ID())
	md.DB.Exec("DELETE FROM discord_states WHERE state = ?", state)

	md.Ctx.Redirect("https://akatsuki.gg", 301)
	return common.SimpleResponse(301, "")
}
