package v1

import (
	"fmt"
	"strconv"

	"github.com/osuAkatsuki/akatsuki-api/common"
	redis "gopkg.in/redis.v5"
)

type hypotheticalRankResponse struct {
	common.ResponseBase
	Rank int `json:"rank"`
}

func HypotheticalRankGET(md common.MethodData) common.CodeMessager {
	modeInt, err := strconv.Atoi(md.Query("mode"))
	if err != nil || modeInt > 3 || modeInt < 0 {
		return common.SimpleResponse(400, "invalid mode")
	}

	mode := modesToReadable[modeInt]

	rx, err := strconv.Atoi(md.Query("rx"))
	if err != nil || rx > 2 || rx < 0 {
		return common.SimpleResponse(400, "invalid relax int")
	}

	performancePoints, err := strconv.Atoi(md.Query("pp"))
	if err != nil || performancePoints < 0 {
		return common.SimpleResponse(400, "invalid performance points")
	}

	var rank int
	var rankErr error
	if rx == 0 {
		rank, rankErr = rankAtPerformancePoints(md.R, mode, performancePoints)
	} else if rx == 1 {
		rank, rankErr = relaxRankAtPerformancePoints(md.R, mode, performancePoints)
	} else if rx == 2 {
		rank, rankErr = autopilotRankAtPerformancePoints(md.R, mode, performancePoints)
	}

	if rankErr != nil {
		md.Err(err)
		return common.SimpleResponse(500, "failed to calculate hypothetical rank")
	}

	resp := hypotheticalRankResponse{
		Rank: rank,
	}
	resp.Code = 200
	return resp
}

func rankAtPerformancePoints(r *redis.Client, mode string, performancePoints int) (int, error) {
	return _rankAtPerformancePoints(r, "ripple:leaderboard:"+mode, performancePoints)
}

func relaxRankAtPerformancePoints(r *redis.Client, mode string, performancePoints int) (int, error) {
	return _rankAtPerformancePoints(r, "ripple:relaxboard:"+mode, performancePoints)
}

func autopilotRankAtPerformancePoints(r *redis.Client, mode string, performancePoints int) (int, error) {
	return _rankAtPerformancePoints(r, "ripple:autoboard:"+mode, performancePoints)
}

func _rankAtPerformancePoints(r *redis.Client, key string, performancePoints int) (int, error) {
	res := r.ZCount(key, fmt.Sprintf("(%i", performancePoints), "inf")
	err := res.Err()
	if err != nil {
		return -1, err
	}

	x := int(res.Val()) + 1
	return x, nil
}
