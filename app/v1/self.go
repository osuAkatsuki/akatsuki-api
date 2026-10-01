package v1

import (
	"database/sql"

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
	FavouriteMode *int `json:"favourite_mode"`
	CustomBadge   struct {
		singleBadge
		Show *bool `json:"show"`
	} `json:"custom_badge"`
	PlayStyle             *int  `json:"play_style"`
	VanillaPPLeaderboards *bool `json:"vanilla_pp_leaderboards"`
	LeaderboardSize       *int  `json:"leaderboard_size"`
	UserTitle             *string `json:"user_title"`
}

// UsersSelfSettingsPOST allows to modify information about the current user.
func UsersSelfSettingsPOST(md common.MethodData) common.CodeMessager {
	var d userSettingsData
	md.Unmarshal(&d)

	// input sanitisation
	if md.User.UserPrivileges&common.UserPrivilegeDonor > 0 {
		d.CustomBadge.Name = common.SanitiseString(d.CustomBadge.Name)
		// d.CustomBadge.Icon = sanitiseIconName(d.CustomBadge.Icon)
	} else {
		d.CustomBadge.singleBadge = singleBadge{}
		d.CustomBadge.Show = nil
	}
	d.FavouriteMode = intPtrIn(0, d.FavouriteMode, 3)

	// Validate user title if provided
	if d.UserTitle != nil && *d.UserTitle == "" {
		// Preserve the stored selection when the settings form sends an empty value.
		d.UserTitle = nil
	} else if d.UserTitle != nil {
		if *d.UserTitle == "donor" {
			*d.UserTitle = "premium"
		}
		// Non-empty title - validate it's in the eligible titles
		var privileges uint64
		err := md.DB.QueryRow("SELECT privileges FROM users WHERE id = ?", md.ID()).Scan(&privileges)
		if err != nil {
			md.Err(err)
			return Err500
		}

		eligibleTitles, err := getEligibleTitles(md, md.ID(), privileges)
		if err != nil {
			md.Err(err)
			return Err500
		}

		// Check if the provided title is in the eligible titles
		titleValid := false
		var selectedTitleID string
		for _, title := range eligibleTitles {
			if title.ID == *d.UserTitle {
				titleValid = true
				selectedTitleID = title.ID // Always use the machine-readable ID for storage
				break
			}
		}

		if !titleValid {
			return common.SimpleResponse(400, "Invalid title selected")
		}

		// Use the normalized title ID (machine-readable)
		*d.UserTitle = selectedTitleID
	}

	q := new(common.UpdateQuery).
		Add("favourite_mode", d.FavouriteMode).
		Add("custom_badge_name", d.CustomBadge.Name).
		Add("custom_badge_icon", d.CustomBadge.Icon).
		Add("show_custom_badge", d.CustomBadge.Show).
		Add("play_style", d.PlayStyle).
		Add("vanilla_pp_leaderboards", d.VanillaPPLeaderboards).
		Add("leaderboard_size", d.LeaderboardSize).
		Add("user_title", d.UserTitle)
	_, err := md.DB.Exec("UPDATE users SET "+q.Fields()+" WHERE id = ?", append(q.Parameters, md.ID())...)
	if err != nil {
		md.Err(err)
		return Err500
	}
	return UsersSelfSettingsGET(md)
}

type eligibleTitle struct {
	ID    string `json:"id"`    // Machine-readable identifier
	Title string `json:"title"` // Human-readable name
}

type userTitleResponse struct {
	ID    string `json:"id"`    // Machine-readable identifier
	Title string `json:"title"` // Human-readable name
}

type userSettingsResponse struct {
	common.ResponseBase
	ID             int                `json:"id"`
	Username       string             `json:"username"`
	Email          string             `json:"email"`
	UserTitle      userTitleResponse  `json:"user_title"`
	EligibleTitles []eligibleTitle    `json:"eligible_titles"`
	userSettingsData
}

// getEligibleTitles determines which titles a user is eligible for based on their privileges and badges.
// The rules are based on the provided template logic:
// - Privilege-based titles: Check if user has specific privilege combinations
// - Badge-based titles: Check if user has specific badges by ID
// - Titles are returned in a specific priority order (to accomodate default title selection)
func getEligibleTitles(md common.MethodData, userID int, privileges uint64) ([]eligibleTitle, error) {
	titles := make([]eligibleTitle, 0)

	userPrivs := common.UserPrivileges(privileges)
	// Staff title eligibility is independent of donor and premium entitlement.
	staffTitlePrivileges := userPrivs | common.UserPrivilegeDonor | common.UserPrivilegePremium

	// Check badges first (they have higher priority)
	rows, err := md.DB.Query("SELECT b.id FROM user_badges ub "+
		"INNER JOIN badges b ON ub.badge = b.id WHERE user = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hasBot := false
	hasDesign := false
	hasScorewatcher := false
	hasChampion := false

	for rows.Next() {
		var badgeID int
		err := rows.Scan(&badgeID)
		if err != nil {
			continue
		}

		if badgeID == 34 {
			hasBot = true
		}

		if badgeID == 101 {
			hasDesign = true
		}

		if badgeID == 86 {
			hasScorewatcher = true
		}

		if badgeID == 67 {
			hasChampion = true
		}
	}

	// Return titles in priority order as specified in the HTML template
	if hasBot {
		titles = append(titles, eligibleTitle{ID: "bot", Title: "CHAT BOT"})
	}

	if staffTitlePrivileges&9437183 == 9437183 {
		titles = append(titles, eligibleTitle{ID: "product_manager", Title: "PRODUCT MANAGER"})
	}

	if staffTitlePrivileges&10743327 == 10743327 {
		titles = append(titles, eligibleTitle{ID: "developer", Title: "PRODUCT DEVELOPER"})
	}

	if hasDesign {
		titles = append(titles, eligibleTitle{ID: "designer", Title: "PRODUCT DESIGNER"})
	}

	if staffTitlePrivileges&9425151 == 9425151 {
		titles = append(titles, eligibleTitle{ID: "community_manager", Title: "COMMUNITY MANAGER"})
	}

	if staffTitlePrivileges&9212159 == 9212159 || staffTitlePrivileges&9175111 == 9175111 {
		titles = append(titles, eligibleTitle{ID: "community_support", Title: "COMMUNITY SUPPORT"})
	}

	if staffTitlePrivileges&10485767 == 10485767 {
		titles = append(titles, eligibleTitle{ID: "event_manager", Title: "EVENT MANAGER"})
	}

	if userPrivs&33554432 == 33554432 {
		titles = append(titles, eligibleTitle{ID: "nqa", Title: "NOMINATION QUALITY ASSURANCE"})
	}

	if staffTitlePrivileges&8388871 == 8388871 {
		titles = append(titles, eligibleTitle{ID: "nominator", Title: "BEATMAP NOMINATOR"})
	}

	if hasScorewatcher {
		titles = append(titles, eligibleTitle{ID: "scorewatcher", Title: "SOCIAL MEDIA MANAGER"})
	}

	if hasChampion {
		titles = append(titles, eligibleTitle{ID: "champion", Title: "AKATSUKI CHAMPION"})
	}

	if userPrivs&common.UserPrivilegePremium == common.UserPrivilegePremium {
		titles = append(titles, eligibleTitle{ID: "premium", Title: "AKATSUKI+"})
	}

	return titles, nil
}

func lookupBuiltInTitle(titleID string) (string, bool) {
	titleMap := map[string]string{
		"bot":               "CHAT BOT",
		"product_manager":   "PRODUCT MANAGER",
		"developer":         "PRODUCT DEVELOPER",
		"designer":          "PRODUCT DESIGNER",
		"community_manager": "COMMUNITY MANAGER",
		"community_support": "COMMUNITY SUPPORT",
		"event_manager":     "EVENT MANAGER",
		"nqa":               "NOMINATION QUALITY ASSURANCE",
		"nominator":         "BEATMAP NOMINATOR",
		"scorewatcher":      "SOCIAL MEDIA MANAGER",
		"champion":          "AKATSUKI CHAMPION",
		"premium":           "AKATSUKI+",
	}
	title, known := titleMap[titleID]
	return title, known
}

func resolveUserTitle(selected sql.NullString, eligible []eligibleTitle) userTitleResponse {
	if selected.Valid && selected.String != "" {
		if selected.String == "donor" {
			selected.String = "premium"
		}
		displayTitle, known := lookupBuiltInTitle(selected.String)
		if !known {
			// Preserve literal custom titles assigned outside self-service settings.
			return userTitleResponse{ID: selected.String, Title: selected.String}
		}
		for _, title := range eligible {
			if title.ID == selected.String {
				return userTitleResponse{ID: selected.String, Title: displayTitle}
			}
		}
	}
	if len(eligible) > 0 {
		return userTitleResponse{ID: eligible[0].ID, Title: eligible[0].Title}
	}
	return userTitleResponse{}
}

// UsersSelfSettingsGET allows to get "sensitive" information about the current user.
func UsersSelfSettingsGET(md common.MethodData) common.CodeMessager {
	var r userSettingsResponse
	var ccb bool
	var privileges uint64
	var userTitleID sql.NullString
	r.Code = 200
	err := md.DB.QueryRow(`
SELECT
	id, username,
	email, favourite_mode,
	show_custom_badge, custom_badge_icon,
	custom_badge_name, can_custom_badge,
	play_style, vanilla_pp_leaderboards,
	leaderboard_size, privileges,
	user_title
FROM users
WHERE id = ?`, md.ID()).Scan(
		&r.ID, &r.Username,
		&r.Email, &r.FavouriteMode,
		&r.CustomBadge.Show, &r.CustomBadge.Icon,
		&r.CustomBadge.Name, &ccb,
		&r.PlayStyle, &r.VanillaPPLeaderboards,
		&r.LeaderboardSize, &privileges,
		&userTitleID,
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

	eligibleTitles, err := getEligibleTitles(md, r.ID, privileges)
	if err != nil {
		md.Err(err)
		return Err500
	} else {
		r.EligibleTitles = eligibleTitles
	}

	r.UserTitle = resolveUserTitle(userTitleID, r.EligibleTitles)

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
