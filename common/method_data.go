package common

import (
	"encoding/json"
	"runtime"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/valyala/fasthttp"
	"golang.org/x/exp/slog"
	"gopkg.in/redis.v5"
)

// MethodData is a struct containing the data passed over to an API method.
type MethodData struct {
	User Token
	DB   *sqlx.DB
	R    *redis.Client
	Ctx  *fasthttp.RequestCtx
}

// ClientIP implements a best effort algorithm to return the real client IP, it parses
// X-Real-IP and X-Forwarded-For in order to work properly with reverse-proxies such us: nginx or haproxy.
func (md MethodData) ClientIP() string {
	clientIP := strings.TrimSpace(string(md.Ctx.Request.Header.Peek("X-Real-Ip")))
	if len(clientIP) > 0 {
		return clientIP
	}
	clientIP = string(md.Ctx.Request.Header.Peek("X-Forwarded-For"))
	if index := strings.IndexByte(clientIP, ','); index >= 0 {
		clientIP = clientIP[0:index]
	}
	clientIP = strings.TrimSpace(clientIP)
	if len(clientIP) > 0 {
		return clientIP
	}
	return md.Ctx.RemoteIP().String()
}

// Err logs an error. If RavenClient is set, it will use the client to report
// the error to sentry, otherwise it will just write the error to stdout.
func (md MethodData) Err(err error) {
	// Generate tags for error
	tags := map[string]string{
		"endpoint": string(md.Ctx.RequestURI()),
		"token":    md.User.Value,
	}

	_err(err, tags, md.Ctx)
}

// Err for peppy API calls
func Err(c *fasthttp.RequestCtx, err error) {
	// Generate tags for error
	tags := map[string]string{
		"endpoint": string(c.RequestURI()),
	}

	_err(err, tags, c)
}

// WSErr is the error function for errors happening in the websockets.
func WSErr(err error) {
	_err(err, map[string]string{
		"endpoint": "/api/v1/ws",
	}, nil)
}

// GenericError is just an error. Can't make a good description.
func GenericError(err error) {
	_err(err, nil, nil)
}

func _err(err error, tags map[string]string, c *fasthttp.RequestCtx) {
	_, file, no, ok := runtime.Caller(2)
	if ok {
		slog.Error("An error occurred", "filename", file, "line", no, "error", err.Error())
	} else {
		slog.Error("An error occurred", "error", err.Error())
	}
}

// ID retrieves the Token's owner user ID.
func (md MethodData) ID() int {
	return md.User.UserID
}

// Query is shorthand for md.C.Query.
func (md MethodData) Query(q string) string {
	return b2s(md.Ctx.QueryArgs().Peek(q))
}

// HasQuery returns true if the parameter is encountered in the querystring.
// It returns true even if the parameter is "" (the case of ?param&etc=etc)
func (md MethodData) HasQuery(q string) bool {
	return md.Ctx.QueryArgs().Has(q)
}

// Unmarshal unmarshals a request's JSON body into an interface.
func (md MethodData) Unmarshal(into interface{}) error {
	return json.Unmarshal(md.Ctx.PostBody(), into)
}

// IsBearer tells whether the current token is a Bearer (oauth) token.
func (md MethodData) IsBearer() bool {
	return md.User.ID == -1
}
