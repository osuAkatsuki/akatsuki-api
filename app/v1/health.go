package v1

import (
	"github.com/osuAkatsuki/akatsuki-api/app/peppy"
	"github.com/osuAkatsuki/akatsuki-api/common"
)

type healthResponse struct {
	common.ResponseBase
}

func HealthGET(md common.MethodData) common.CodeMessager {
	var r healthResponse

	err := peppy.R.Ping().Err()
	if err != nil {
		r.Code = 500
		r.Message = "redis error"
		return r
	}

	err = md.DB.Ping()
	if err != nil {
		r.Code = 500
		r.Message = "database error"
		return r
	}

	r.Code = 200
	r.Message = "healthy"
	return r
}
