package v1

import (
	"strings"

	"github.com/osuAkatsuki/akatsuki-api/common"
)

type donorInfoResponse struct {
	common.ResponseBase
	HasDonor   bool                 `json:"has_donor"`
	HasPremium bool                 `json:"has_premium"`
	Expiration common.UnixTimestamp `json:"expiration"`
}

// UsersSelfDonorInfoGET returns information about the users' donor status
func UsersSelfDonorInfoGET(md common.MethodData) common.CodeMessager {
	var r donorInfoResponse
	var privileges uint64
	err := md.DB.QueryRow("SELECT privileges, donor_expire FROM users WHERE id = ?", md.ID()).
		Scan(&privileges, &r.Expiration)
	if err != nil {
		md.Err(err)
		return Err500
	}
	r.HasDonor = common.UserPrivileges(privileges)&common.UserPrivilegeDonor > 0
	r.HasPremium = common.UserPrivileges(privileges)&common.UserPrivilegePremium > 0
	r.Code = 200
	return r
}

type favouriteModeResponse struct {
	common.ResponseBase
	FavouriteMode int `json:"favourite_mode"`
}

// UsersSelfFavouriteModeGET gets the current user's favourite mode
func UsersSelfFavouriteModeGET(md common.MethodData) common.CodeMessager {
	var f favouriteModeResponse
	f.Code = 200
	if md.ID() == 0 {
		return f
	}
	err := md.DB.QueryRow("SELECT favourite_mode FROM users WHERE id = ?", md.ID()).
		Scan(&f.FavouriteMode)
	if err != nil {
		md.Err(err)
		return Err500
	}
	return f
}

type userSettingsData struct {
	UsernameAKA   *string `json:"username_aka"`
	FavouriteMode *int    `json:"favourite_mode"`
	CustomBadge   struct {
		singleBadge
		Show *bool `json:"show"`
	} `json:"custom_badge"`
	PlayStyle             *int  `json:"play_style"`
	VanillaPPLeaderboards *bool `json:"vanilla_pp_leaderboards"`
}

// UsersSelfSettingsPOST allows to modify information about the current user.
func UsersSelfSettingsPOST(md common.MethodData) common.CodeMessager {
	var d userSettingsData
	md.Unmarshal(&d)

	aka := strings.TrimSpace(*d.UsernameAKA)
	if aka == "" {
		*d.UsernameAKA = ""
	}

	// input sanitisation
	if md.User.UserPrivileges&common.UserPrivilegeDonor > 0 {
		d.CustomBadge.Name = common.SanitiseString(d.CustomBadge.Name)
		// d.CustomBadge.Icon = sanitiseIconName(d.CustomBadge.Icon)
	} else {
		d.CustomBadge.singleBadge = singleBadge{}
		d.CustomBadge.Show = nil
	}
	d.FavouriteMode = intPtrIn(0, d.FavouriteMode, 3)

	q := new(common.UpdateQuery).
		Add("username_aka", d.UsernameAKA).
		Add("favourite_mode", d.FavouriteMode).
		Add("custom_badge_name", d.CustomBadge.Name).
		Add("custom_badge_icon", d.CustomBadge.Icon).
		Add("show_custom_badge", d.CustomBadge.Show).
		Add("play_style", d.PlayStyle).
		Add("vanilla_pp_leaderboards", d.VanillaPPLeaderboards)
	_, err := md.DB.Exec("UPDATE users SET "+q.Fields()+" WHERE id = ?", append(q.Parameters, md.ID())...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	return UsersSelfSettingsGET(md)
}

type userSettingsResponse struct {
	common.ResponseBase
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	userSettingsData
}

// UsersSelfSettingsGET allows to get "sensitive" information about the current user.
func UsersSelfSettingsGET(md common.MethodData) common.CodeMessager {
	var r userSettingsResponse
	var ccb bool
	r.Code = 200
	err := md.DB.QueryRow(`
SELECT
	id, username,
	email, username_aka, favourite_mode,
	show_custom_badge, custom_badge_icon,
	custom_badge_name, can_custom_badge,
	play_style, vanilla_pp_leaderboards
FROM users
WHERE id = ?`, md.ID()).Scan(
		&r.ID, &r.Username,
		&r.Email, &r.UsernameAKA, &r.FavouriteMode,
		&r.CustomBadge.Show, &r.CustomBadge.Icon,
		&r.CustomBadge.Name, &ccb,
		&r.PlayStyle, &r.VanillaPPLeaderboards,
	)
	if err != nil {
		md.Err(err)
		return Err500
	}
	if !ccb {
		r.CustomBadge = struct {
			singleBadge
			Show *bool `json:"show"`
		}{}
	}
	return r
}

func intPtrIn(x int, y *int, z int) *int {
	if y == nil {
		return nil
	}
	if *y > z {
		return nil
	}
	if *y < x {
		return nil
	}
	return y
}
