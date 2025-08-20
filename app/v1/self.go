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
	if d.UserTitle != nil {
		// Get eligible titles to validate
		var privileges uint64
		err := md.DB.QueryRow("SELECT privileges FROM users WHERE id = ?", md.ID()).Scan(&privileges)
		if err != nil {
			md.Err(err)
			return Err500
		}

		eligibleTitles, err := getEligibleTitles(md, privileges)
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
// Titles are returned in a specific priority order (to accomodate default title selection)
func getEligibleTitles(md common.MethodData, privileges uint64) ([]eligibleTitle, error) {
	var titles []eligibleTitle

	// Check privileges
	userPrivs := common.UserPrivileges(privileges)

	// Check badges first (they have higher priority)
	rows, err := md.DB.Query("SELECT b.id FROM user_badges ub "+
		"INNER JOIN badges b ON ub.badge = b.id WHERE user = ?", md.ID())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Track which badges the user has
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

		// Bot badge (ID 34)
		if badgeID == 34 {
			hasBot = true
		}

		// Design badge (ID 101)
		if badgeID == 101 {
			hasDesign = true
		}

		// Scorewatcher badge (ID 86)
		if badgeID == 86 {
			hasScorewatcher = true
		}

		// Champion badge (ID 67)
		if badgeID == 67 {
			hasChampion = true
		}
	}

	// Return titles in priority order as specified in the HTML template
	// 1. Bot (highest priority)
	if hasBot {
		titles = append(titles, eligibleTitle{ID: "bot", Title: "CHAT BOT"})
	}

	// 2. Product Manager
	if userPrivs&9437183 > 0 {
		titles = append(titles, eligibleTitle{ID: "product_manager", Title: "PRODUCT MANAGER"})
	}

	// 3. Developer
	if userPrivs&10743327 > 0 {
		titles = append(titles, eligibleTitle{ID: "developer", Title: "PRODUCT DEVELOPER"})
	}

	// 4. Designer
	if hasDesign {
		titles = append(titles, eligibleTitle{ID: "designer", Title: "PRODUCT DESIGNER"})
	}

	// 5. Community Manager
	if userPrivs&9425151 > 0 {
		titles = append(titles, eligibleTitle{ID: "community_manager", Title: "COMMUNITY MANAGER"})
	}

	// 6. Community Support (Accounts)
	if userPrivs&9212159 > 0 {
		titles = append(titles, eligibleTitle{ID: "community_support", Title: "COMMUNITY SUPPORT"})
	}

	// 7. Community Support (Support)
	if userPrivs&9175111 > 0 {
		titles = append(titles, eligibleTitle{ID: "community_support", Title: "COMMUNITY SUPPORT"})
	}

	// 8. Event Manager
	if userPrivs&10485767 > 0 {
		titles = append(titles, eligibleTitle{ID: "event_manager", Title: "EVENT MANAGER"})
	}

	// 9. NQA
	if userPrivs&33554432 > 0 {
		titles = append(titles, eligibleTitle{ID: "nqa", Title: "NOMINATION QUALITY ASSURANCE"})
	}

	// 10. Nominator
	if userPrivs&8388871 > 0 {
		titles = append(titles, eligibleTitle{ID: "nominator", Title: "BEATMAP NOMINATOR"})
	}

	// 11. Scorewatcher
	if hasScorewatcher {
		titles = append(titles, eligibleTitle{ID: "scorewatcher", Title: "SOCIAL MEDIA MANAGER"})
	}

	// 12. Champion
	if hasChampion {
		titles = append(titles, eligibleTitle{ID: "champion", Title: "AKATSUKI CHAMPION"})
	}

	// 13. Premium
	if userPrivs&common.UserPrivilegePremium > 0 {
		titles = append(titles, eligibleTitle{ID: "premium", Title: "AKATSUKI+"})
	}

	// 14. Donor (lowest priority)
	if userPrivs&common.UserPrivilegeDonor > 0 {
		titles = append(titles, eligibleTitle{ID: "donor", Title: "SUPPORTER"})
	}

	return titles, nil
}

// getTitleFromID converts a machine-readable title ID to human-readable title
func getTitleFromID(titleID string) string {
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
		"donor":             "SUPPORTER",
	}

	if title, exists := titleMap[titleID]; exists {
		return title
	}
	return titleID // Return ID if not found (fallback)
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

	// Get eligible titles
	eligibleTitles, err := getEligibleTitles(md, privileges)
	if err != nil {
		md.Err(err)
		return Err500
	} else {
		r.EligibleTitles = eligibleTitles
	}

	// Set the UserTitle struct from the stored ID
	if userTitleID.Valid && userTitleID.String != "" {
		r.UserTitle = userTitleResponse{
			ID:    userTitleID.String,
			Title: getTitleFromID(userTitleID.String),
		}
	} else if len(r.EligibleTitles) > 0 {
		// If user_title is empty or null, set it to the first eligible title if available
		r.UserTitle = userTitleResponse{
			ID:    r.EligibleTitles[0].ID,
			Title: r.EligibleTitles[0].Title,
		}
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
