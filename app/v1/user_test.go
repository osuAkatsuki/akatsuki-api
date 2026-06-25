package v1

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/osuAkatsuki/akatsuki-api/common"
)

func TestParseUserFullIDs(t *testing.T) {
	ids, errResp := parseUserFullIDs([][]byte{[]byte("2"), []byte("2"), []byte("5")})
	if errResp != nil {
		t.Fatalf("parseUserFullIDs returned error response: %#v", errResp)
	}
	if !reflect.DeepEqual(ids, []int{2, 5}) {
		t.Fatalf("parseUserFullIDs returned %v, want [2 5]", ids)
	}

	_, errResp = parseUserFullIDs([][]byte{[]byte("abc")})
	if errResp == nil {
		t.Fatal("parseUserFullIDs accepted an invalid user ID")
	}

	rawIDs := make([][]byte, maxUserFullBulkIDs+1)
	for i := range rawIDs {
		rawIDs[i] = []byte("1")
	}
	_, errResp = parseUserFullIDs(rawIDs)
	if errResp == nil {
		t.Fatal("parseUserFullIDs accepted more than 50 user IDs")
	}
}

func TestUserFullResponseShapes(t *testing.T) {
	singleResponse := userFullResponse{
		ResponseBase: common.ResponseBase{Code: 200},
		userFullData: userFullData{
			userData: userData{
				ID:      1,
				Country: "CA",
			},
			CountryName: "Canada",
		},
	}

	singleJSON, err := json.Marshal(singleResponse)
	if err != nil {
		t.Fatal(err)
	}
	var single map[string]interface{}
	if err := json.Unmarshal(singleJSON, &single); err != nil {
		t.Fatal(err)
	}
	if _, ok := single["users"]; ok {
		t.Fatal("single full user response unexpectedly contains a users list")
	}
	if single["id"] != float64(1) {
		t.Fatalf("single full user response id = %v, want 1", single["id"])
	}
	if single["country_name"] != "Canada" {
		t.Fatalf("single full user response country_name = %v, want Canada", single["country_name"])
	}

	multiResponse := userFullMultiResponse{
		ResponseBase: common.ResponseBase{Code: 200},
		Users: []userFullData{
			{
				userData: userData{
					ID:      1,
					Country: "CA",
				},
				CountryName: "Canada",
			},
		},
	}

	multiJSON, err := json.Marshal(multiResponse)
	if err != nil {
		t.Fatal(err)
	}
	var multi map[string]interface{}
	if err := json.Unmarshal(multiJSON, &multi); err != nil {
		t.Fatal(err)
	}
	users, ok := multi["users"].([]interface{})
	if !ok || len(users) != 1 {
		t.Fatalf("multi full user response users = %v, want one user", multi["users"])
	}
	user, ok := users[0].(map[string]interface{})
	if !ok {
		t.Fatalf("multi full user response item = %T, want object", users[0])
	}
	if _, ok := user["code"]; ok {
		t.Fatal("bulk full user item unexpectedly contains a code field")
	}
	if user["country_name"] != "Canada" {
		t.Fatalf("bulk full user country_name = %v, want Canada", user["country_name"])
	}
}
