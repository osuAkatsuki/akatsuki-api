package app

import (
	"crypto/md5"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/exp/slog"

	"github.com/jmoiron/sqlx"
	"github.com/osuAkatsuki/akatsuki-api/common"
)

// GetTokenFull retrieves an user ID and their token privileges knowing their API token.
func GetTokenFull(token string, db *sqlx.DB) (common.Token, bool) {
	var (
		t             common.Token
		tokenPrivsRaw uint64
		userPrivsRaw  uint64
		priv8         bool
	)
	err := db.QueryRow(`SELECT
	t.id, t.user, t.privileges, t.private, u.privileges
FROM tokens t
LEFT JOIN users u ON u.id = t.user
WHERE token = ? LIMIT 1`,
		fmt.Sprintf("%x", md5.Sum([]byte(token)))).
		Scan(
			&t.ID, &t.UserID, &tokenPrivsRaw, &priv8, &userPrivsRaw,
		)
	updateTokens <- t.ID
	if priv8 {
		// all privileges, they'll get removed by canOnly anyway.
		tokenPrivsRaw = (common.PrivilegeBeatmap << 1) - 1
	}
	t.UserPrivileges = common.UserPrivileges(userPrivsRaw)
	t.TokenPrivileges = common.Privileges(tokenPrivsRaw).CanOnly(t.UserPrivileges)
	switch {
	case err == sql.ErrNoRows:
		return common.Token{}, false
	case err != nil:
		panic(err)
	default:
		t.Value = token
		return t, true
	}
}

var updateTokens = make(chan int, 100)

func tokenUpdater(db *sqlx.DB) {
	// A cron which updates the last_updated field of tokens every 10 seconds.
	// This is done for the purpose of invalidating tokens which are no longer in use.
	// And is used to improve performance by not having to update the database for every request.
	for {
		// prepare a queue of tokens to update.
		tokensToUpdate := make([]int, 0, 50)
	AwaitLoop:
		for {
			// if we got ten, move on and update
			if len(tokensToUpdate) >= 50 {
				break
			}
			// if we ain't got any, add what we get straight from updateTokens
			if len(tokensToUpdate) == 0 {
				x := <-updateTokens
				tokensToUpdate = append(tokensToUpdate, x)
				continue
			}

			// otherwise, wait from updateTokens with a timeout of 10 seconds
			select {
			case x := <-updateTokens:
				tokensToUpdate = append(tokensToUpdate, x)
			case <-time.After(10 * time.Second):
				break AwaitLoop
			}
		}

		q, a, _ := sqlx.In("UPDATE tokens SET last_updated = ? WHERE id IN (?)", time.Now().Unix(), tokensToUpdate)

		q = db.Rebind(q)
		_, err := db.Exec(q, a...)
		if err != nil {
			slog.Error("Error updating tokens", "error", err.Error())
		}
	}
}

// BearerToken parses a Token given in the Authorization header, with the
// Bearer prefix.
func BearerToken(token string, db *sqlx.DB) (common.Token, bool) {
	var x struct {
		Scope string
		Extra int
	}
	db.Get(&x, "SELECT scope, extra FROM osin_access WHERE access_token = ? LIMIT 1", fmt.Sprintf("%x", sha256.Sum256([]byte(token))))
	if x.Extra == 0 {
		return common.Token{}, false
	}

	var privs uint64
	db.Get(&privs, "SELECT privileges FROM users WHERE id = ? LIMIT 1", x.Extra)

	var t common.Token
	t.ID = -1
	t.UserID = x.Extra
	t.Value = token
	t.UserPrivileges = common.UserPrivileges(privs)
	t.TokenPrivileges = common.OAuthPrivileges(x.Scope).CanOnly(t.UserPrivileges)

	return t, true
}
