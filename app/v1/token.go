package v1

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/osuAkatsuki/akatsuki-api/common"
)

// TokenSelfDeletePOST deletes the token the user is connecting with.
func TokenSelfDeletePOST(md common.MethodData) common.CodeMessager {
	if md.ID() == 0 {
		return common.SimpleResponse(400, "How should we delete your token if you haven't even given us one?!")
	}
	var err error
	if md.IsBearer() {
		_, err = md.DB.Exec("DELETE FROM osin_access WHERE access_token = ? LIMIT 1",
			fmt.Sprintf("%x", sha256.Sum256([]byte(md.User.Value))))
	} else {
		_, err = md.DB.Exec("DELETE FROM tokens WHERE token = ? LIMIT 1",
			fmt.Sprintf("%x", md5.Sum([]byte(md.User.Value))))
	}
	if err != nil {
		md.Err(err)
		return Err500
	}
	return common.SimpleResponse(200, "Bye!")
}

type token struct {
	ID          int                  `json:"id"`
	Privileges  uint64               `json:"privileges"`
	Description string               `json:"description"`
	LastUpdated common.UnixTimestamp `json:"last_updated"`
}
type tokenResponse struct {
	common.ResponseBase
	Tokens []token `json:"tokens"`
}

// TokenGET retrieves a list listing all the user's public tokens.
func TokenGET(md common.MethodData) common.CodeMessager {
	wc := common.Where("user = ? AND private = 0", strconv.Itoa(md.ID()))
	if md.Query("id") != "" {
		wc.Where("id = ?", md.Query("id"))
	}
	rows, err := md.DB.Query("SELECT id, privileges, description, last_updated FROM tokens "+
		wc.Clause+common.Paginate(md.Query("p"), md.Query("l"), 50), wc.Params...)

	if err != nil {
		return Err500
	}
	var r tokenResponse
	for rows.Next() {
		var t token
		err = rows.Scan(&t.ID, &t.Privileges, &t.Description, &t.LastUpdated)
		if err != nil {
			md.Err(err)
			continue
		}
		r.Tokens = append(r.Tokens, t)
	}
	r.Code = 200
	return r
}

type oauthClient struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OwnerID int    `json:"owner_id"`
	Avatar  string `json:"avatar"`
}

// Scan scans the extra in the mysql table into Name, OwnerID and Avatar.
func (o *oauthClient) Scan(src interface{}) error {
	var s []byte
	switch x := src.(type) {
	case string:
		s = []byte(x)
	case []byte:
		s = x
	default:
		return errors.New("Can't scan non-string")
	}

	var vals [3]string
	err := json.Unmarshal(s, &vals)
	if err != nil {
		return err
	}

	o.Name = vals[0]
	o.OwnerID, _ = strconv.Atoi(vals[1])
	o.Avatar = vals[2]

	return nil
}

type bearerToken struct {
	Client     oauthClient       `json:"client"`
	Scope      string            `json:"scope"`
	Privileges common.Privileges `json:"privileges"`
	Created    time.Time         `json:"created"`
}

type tokenSingleResponse struct {
	common.ResponseBase
	token
}

type bearerTokenSingleResponse struct {
	common.ResponseBase
	bearerToken
}

// TokenSelfGET retrieves information about the token the user is connecting with.
func TokenSelfGET(md common.MethodData) common.CodeMessager {
	if md.ID() == 0 {
		return common.SimpleResponse(404, "How are we supposed to find the token you're using if you ain't even using one?!")
	}
	if md.IsBearer() {
		return getBearerToken(md)
	}
	var r tokenSingleResponse
	// md.User.ID = token id, userid would have been md.User.UserID. what a clusterfuck
	err := md.DB.QueryRow("SELECT id, privileges, description, last_updated FROM tokens WHERE id = ?", md.User.ID).Scan(
		&r.ID, &r.Privileges, &r.Description, &r.LastUpdated,
	)
	if err != nil {
		md.Err(err)
		return Err500
	}
	r.Code = 200
	return r
}

func getBearerToken(md common.MethodData) common.CodeMessager {
	var b bearerTokenSingleResponse
	err := md.DB.
		QueryRow(`
			SELECT t.scope, t.created_at, c.id, c.extra
			FROM osin_access t INNER JOIN osin_client c
			WHERE t.access_token = ?
		`, fmt.Sprintf("%x", sha256.Sum256([]byte(md.User.Value)))).Scan(
		&b.Scope, &b.Created, &b.Client.ID, &b.Client,
	)
	if err != nil {
		md.Err(err)
		return Err500
	}
	b.Code = 200
	b.Privileges = md.User.TokenPrivileges
	return b
}
