package v1

import (
	"database/sql"
	"testing"
)

func TestResolveUserTitle(t *testing.T) {
	eligible := []eligibleTitle{
		{ID: "developer", Title: "PRODUCT DEVELOPER"},
		{ID: "donor", Title: "SUPPORTER"},
	}

	tests := []struct {
		name     string
		selected sql.NullString
		eligible []eligibleTitle
		want     userTitleResponse
	}{
		{"valid selection", sql.NullString{String: "donor", Valid: true}, eligible, userTitleResponse{ID: "donor", Title: "SUPPORTER"}},
		{"expired selection", sql.NullString{String: "premium", Valid: true}, eligible, userTitleResponse{ID: "developer", Title: "PRODUCT DEVELOPER"}},
		{"automatic default", sql.NullString{}, eligible, userTitleResponse{ID: "developer", Title: "PRODUCT DEVELOPER"}},
		{"empty selection uses default", sql.NullString{String: "", Valid: true}, eligible, userTitleResponse{ID: "developer", Title: "PRODUCT DEVELOPER"}},
		{"custom title", sql.NullString{String: "BALLIN", Valid: true}, eligible, userTitleResponse{ID: "BALLIN", Title: "BALLIN"}},
		{"custom title without eligible roles", sql.NullString{String: "custom title", Valid: true}, nil, userTitleResponse{ID: "custom title", Title: "custom title"}},
		{"custom title is case sensitive", sql.NullString{String: "Premium", Valid: true}, nil, userTitleResponse{ID: "Premium", Title: "Premium"}},
		{"custom title preserves trailing space", sql.NullString{String: "premium ", Valid: true}, nil, userTitleResponse{ID: "premium ", Title: "premium "}},
		{"expired selection without eligible roles", sql.NullString{String: "premium", Valid: true}, nil, userTitleResponse{}},
		{"no eligible title", sql.NullString{}, nil, userTitleResponse{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveUserTitle(tt.selected, tt.eligible); got != tt.want {
				t.Errorf("resolveUserTitle() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
