package app

import (
	"encoding/json"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestWriteRequestPolicy(t *testing.T) {
	tests := []struct {
		name, origin, contentType, body string
		want                            int
	}{
		{"website JSON", "https://akatsuki.gg", "application/json; charset=utf-8", "{}", 0},
		{"next website JSON", "https://next.akatsuki.gg", "application/json", "{}", 0},
		{"CLI JSON", "", "application/json", "{}", 0},
		{"empty update", "https://akatsuki.gg", "", "", 0},
		{"third-party origin", "https://example.org", "application/json", "{}", fasthttp.StatusForbidden},
		{"opaque origin", "null", "application/json", "{}", fasthttp.StatusForbidden},
		{"non-JSON body", "https://akatsuki.gg", "text/plain", "hello", fasthttp.StatusUnsupportedMediaType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ctx fasthttp.RequestCtx
			ctx.Request.Header.Set("Origin", tt.origin)
			ctx.Request.Header.SetContentType(tt.contentType)
			ctx.Request.SetBodyString(tt.body)
			if got, _ := validateWriteRequest(&ctx, "akatsuki.gg"); got != tt.want {
				t.Fatalf("got status %d, want %d", got, tt.want)
			}
		})
	}
}

func TestResponseUsesJSON(t *testing.T) {
	var ctx fasthttp.RequestCtx
	ctx.QueryArgs().Set("callback", "unused")
	mkjson(&ctx, map[string]int{"value": 1})
	if string(ctx.Response.Header.ContentType()) != "application/json; charset=utf-8" ||
		!json.Valid(ctx.Response.Body()) {
		t.Fatalf("got content type %s and body %s", ctx.Response.Header.ContentType(), ctx.Response.Body())
	}
}
