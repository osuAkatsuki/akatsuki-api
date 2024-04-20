package v1

import (
	"time"

	"github.com/osuAkatsuki/akatsuki-api/common"
)

func rapLog(md common.MethodData, message string) {
	ua := string(md.Ctx.UserAgent())
	if len(ua) > 20 {
		ua = ua[:20] + "…"
	}
	through := "API"
	if ua != "" {
		through += " (" + ua + ")"
	}

	_, err := md.DB.Exec("INSERT INTO rap_logs(userid, text, datetime, through) VALUES (?, ?, ?, ?)",
		md.User.UserID, message, time.Now().Unix(), through)
	if err != nil {
		md.Err(err)
	}
}
