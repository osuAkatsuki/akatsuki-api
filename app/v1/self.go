package v1

import (
	"strings"

	"github.com/osuAkatsuki/akatsuki-api/common"
	semanticiconsugc "zxq.co/ripple/semantic-icons-ugc"
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
	err := md.DB.QueryRow("SELECT priv, donor_end FROM users WHERE id = ?", md.ID()).
		Scan(&privileges, &r.Expiration)
	if err != nil {
		md.Err(err)
		return Err500
	}
	r.HasDonor = common.Privileges(privileges) & common.SUPPORTER > 0
	r.HasPremium = common.Privileges(privileges) & common.PREMIUM > 0
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
	err := md.DB.QueryRow("SELECT preferred_mode FROM users WHERE id = ?", md.ID()).
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
	PlayStyle *int `json:"play_style"`
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
	if md.User.UserPrivileges & common.DONATOR > 0 {
		d.CustomBadge.Name = common.SanitiseString(d.CustomBadge.Name)
		d.CustomBadge.Icon = sanitiseIconName(d.CustomBadge.Icon)
	} else {
		d.CustomBadge.singleBadge = singleBadge{}
		d.CustomBadge.Show = nil
	}
	d.FavouriteMode = intPtrIn(0, d.FavouriteMode, 3)

	q := new(common.UpdateQuery).
		Add("u.username_aka", d.UsernameAKA).
		Add("u.preferred_mode", d.FavouriteMode).
		Add("u.custom_badge_name", d.CustomBadge.Name).
		Add("u.custom_badge_icon", d.CustomBadge.Icon).
		Add("u.play_style", d.PlayStyle)
	_, err := md.DB.Exec("UPDATE users u SET "+q.Fields()+" WHERE u.id = ?", append(q.Parameters, md.ID())...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	return UsersSelfSettingsGET(md)
}

func sanitiseIconName(s string) string {
	classes := strings.Split(s, " ")
	n := make([]string, 0, len(classes))
	for _, c := range classes {
		if !in(c, n) && in(c, semanticiconsugc.SaneIcons) {
			n = append(n, c)
		}
	}
	return strings.Join(n, " ")
}

func in(a string, b []string) bool {
	for _, x := range b {
		if x == a {
			return true
		}
	}
	return false
}

type userSettingsResponse struct {
	common.ResponseBase
	ID       int    `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Flags    uint   `json:"flags"`
	userSettingsData
}

// UsersSelfSettingsGET allows to get "sensitive" information about the current user.
func UsersSelfSettingsGET(md common.MethodData) common.CodeMessager {
	var r userSettingsResponse
	r.Code = 200
	err := md.DB.QueryRow(`
SELECT
	u.id, u.username,
	u.email, s.username_aka, u.favourite_mode,
	u.custom_badge_icon,
	u.custom_badge_name,
	u.play_style
FROM users u
WHERE u.id = ?`, md.ID()).Scan(
		&r.ID, &r.Username,
		&r.Email, &r.UsernameAKA, &r.FavouriteMode,
		&r.CustomBadge.Icon,
		&r.CustomBadge.Name,
		&r.PlayStyle,
	)
	if err != nil {
		md.Err(err)
		return Err500
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
