package app

import (
	"encoding/json"
	"mime"
	"reflect"
	"strings"
	"unsafe"

	"golang.org/x/exp/slog"

	"github.com/osuAkatsuki/akatsuki-api/common"
	"github.com/valyala/fasthttp"
)

// Method wraps an API method to a HandlerFunc.
func Method(f func(md common.MethodData) common.CodeMessager, privilegesNeeded ...int) fasthttp.RequestHandler {
	return func(c *fasthttp.RequestCtx) {
		initialCaretaker(c, f, privilegesNeeded...)
	}
}

func initialCaretaker(c *fasthttp.RequestCtx, f func(md common.MethodData) common.CodeMessager, privilegesNeeded ...int) {
	c.Response.Header.Set("X-Content-Type-Options", "nosniff")
	c.Response.Header.Set("X-Frame-Options", "DENY")
	if c.IsPost() {
		if code, message := validateWriteRequest(c, common.GetSettings().APP_DOMAIN); code != 0 {
			c.SetStatusCode(code)
			mkjson(c, common.SimpleResponse(code, message))
			return
		}
	}
	qa := c.Request.URI().QueryArgs()
	var token string
	var bearerToken bool
	switch {
	case len(c.Request.Header.CookieBytes([]byte("X-Ripple-Token"))) > 0:
		token = string(c.Request.Header.CookieBytes([]byte("X-Ripple-Token")))
	case len(c.Request.Header.Peek("X-Ripple-Token")) > 0:
		token = string(c.Request.Header.Peek("X-Ripple-Token"))
	case strings.HasPrefix(string(c.Request.Header.Peek("Authorization")), "Bearer "):
		token = strings.TrimPrefix(string(c.Request.Header.Peek("Authorization")), "Bearer ")
		bearerToken = true
	case len(qa.Peek("token")) > 0:
		token = string(qa.Peek("token"))
	case len(qa.Peek("k")) > 0:
		token = string(qa.Peek("k"))
	default:
		token = string(c.Request.Header.Cookie("rt"))
	}

	md := common.MethodData{
		DB:  db,
		Ctx: c,
		R:   red,
	}
	if token != "" {
		var (
			tokenReal common.Token
			exists    bool
		)
		if bearerToken {
			tokenReal, exists = BearerToken(token, db)
		} else {
			tokenReal, exists = GetTokenFull(token, db)
		}
		if exists {
			md.User = tokenReal
		}
	}

	missingPrivileges := 0
	for _, privilege := range privilegesNeeded {
		if uint64(md.User.TokenPrivileges)&uint64(privilege) == 0 {
			missingPrivileges |= privilege
		}
	}
	if missingPrivileges != 0 {
		slog.Error(
			"Denied access due to missing privileges",
			"tokenHas", md.User.TokenPrivileges,
			"userHas", md.User.UserPrivileges,
			"missing", missingPrivileges,
			"userID", md.User.UserID,
			"route", string(c.Request.URI().Path()),
		)
		c.SetStatusCode(401)
		mkjson(c, common.SimpleResponse(401, "Unauthorized."))
		return
	}

	resp := f(md)
	c.SetStatusCode(resp.GetCode())

	mkjson(c, resp)
}

func validateWriteRequest(c *fasthttp.RequestCtx, domain string) (int, string) {
	// Non-browser clients omit Origin. Browsers include it on cross-origin POSTs.
	if origin := string(c.Request.Header.Peek("Origin")); origin != "" {
		if domain == "" || (origin != "https://"+domain && origin != "https://next."+domain) {
			return fasthttp.StatusForbidden, "Origin is not allowed."
		}
	}
	if len(c.PostBody()) != 0 {
		mediaType, _, err := mime.ParseMediaType(string(c.Request.Header.ContentType()))
		if err != nil || mediaType != "application/json" {
			return fasthttp.StatusUnsupportedMediaType, "Expected application/json."
		}
	}
	return 0, ""
}

func mkjson(c *fasthttp.RequestCtx, data interface{}) {
	c.Response.Header.SetContentType("application/json; charset=utf-8")
	exported, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		slog.Error("Error marshalling JSON", "error", err.Error())
		exported = []byte(`{ "code": 500, "message": "An unexpected error occurred." }`)
	}
	c.SetBody(exported)
}

// b2s converts byte slice to a string without memory allocation.
// See https://groups.google.com/forum/#!msg/Golang-Nuts/ENgbUzYvCuU/90yGx7GUAgAJ .
//
// Note it may break if string and/or slice header will change
// in the future go versions.
func b2s(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// s2b converts string to a byte slice without memory allocation.
//
// Note it may break if string and/or slice header will change
// in the future go versions.
func s2b(s string) []byte {
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	bh := reflect.SliceHeader{
		Data: sh.Data,
		Len:  sh.Len,
		Cap:  sh.Len,
	}
	return *(*[]byte)(unsafe.Pointer(&bh))
}
