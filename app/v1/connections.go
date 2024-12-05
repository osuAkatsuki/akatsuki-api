package v1

import (
	"database/sql"
	"encoding/json"

	"github.com/go-resty/resty/v2"
	"github.com/osuAkatsuki/akatsuki-api/common"
)

func DiscordCallbackGET(md common.MethodData) common.CodeMessager {
	code := md.Query("code")
	if code == "" {
		return ErrBadJSON
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

	md.Ctx.Redirect("https://akatsuki.gg", 301)
	return common.SimpleResponse(301, "")
}
