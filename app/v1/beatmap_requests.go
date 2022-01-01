package v1

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"github.com/osuAkatsuki/akatsuki-api/common"
	"github.com/osuAkatsuki/akatsuki-api/limit"
)

type rankRequestsStatusResponse struct {
	common.ResponseBase
	QueueSize       int        `json:"queue_size"`
	MaxPerUser      int        `json:"max_per_user"`
	Submitted       int        `json:"submitted"`
	SubmittedByUser *int       `json:"submitted_by_user,omitempty"`
	CanSubmit       *bool      `json:"can_submit,omitempty"`
	NextExpiration  *time.Time `json:"next_expiration"`
}

// BeatmapRankRequestsStatusGET gets the current status for beatmap ranking requests.
func BeatmapRankRequestsStatusGET(md common.MethodData) common.CodeMessager {
	c := common.GetConf()
	rows, err := md.DB.Query("SELECT userid, time FROM rank_requests WHERE time > ? ORDER BY id ASC LIMIT "+strconv.Itoa(c.RankQueueSize), time.Now().Add(-time.Hour*24).Unix())
	if err != nil {
		md.Err(err)
		return Err500
	}
	var r rankRequestsStatusResponse
	isFirst := true
	for rows.Next() {
		var (
			user      int
			timestamp common.UnixTimestamp
		)
		err := rows.Scan(&user, &timestamp)
		if err != nil {
			md.Err(err)
			continue
		}
		// if the user submitted this rank request, increase the number of
		// rank requests submitted by this user
		if user == md.ID() && r.SubmittedByUser != nil {
			(*r.SubmittedByUser)++
		}
		// also, if this is the first result, it means it will be the next to
		// expire.
		if isFirst {
			x := time.Time(timestamp)
			r.NextExpiration = &x
			isFirst = false
		}
		r.Submitted++
	}
	r.QueueSize = c.RankQueueSize
	r.MaxPerUser = c.BeatmapRequestsPerUser
	x := r.Submitted < r.QueueSize && *r.SubmittedByUser < r.MaxPerUser
	r.CanSubmit = &x
	r.Code = 200
	return r
}

type submitRequestData struct {
	ID    int `json:"id"`
}

// BeatmapRankRequestsSubmitPOST submits a new beatmap for ranking approval.
func BeatmapRankRequestsSubmitPOST(md common.MethodData) common.CodeMessager {
	var d submitRequestData
	err := md.Unmarshal(&d)
	if err != nil {
		return ErrBadJSON
	}
	// check json data is present
	if d.ID == 0 {
		return ErrMissingField("id")
	}

	// you've been rate limited
	if !limit.NonBlockingRequest("rankrequest:u:"+strconv.Itoa(md.ID()), 5) {
		return common.SimpleResponse(429, "You may only try to request 5 beatmaps per minute.")
	}

	// find out from BeatmapRankRequestsStatusGET if we can submit beatmaps.
	statusRaw := BeatmapRankRequestsStatusGET(md)
	status, ok := statusRaw.(rankRequestsStatusResponse)
	if !ok {
		// if it's not a rankRequestsStatusResponse, it means it's an error
		return statusRaw
	}
	if !*status.CanSubmit {
		return common.SimpleResponse(403, "It's not possible to do a rank request at this time.")
	}

	var ranked int
	err = md.DB.QueryRow("SELECT status FROM beatmaps WHERE id = ? LIMIT 1", d.ID).Scan(&ranked)
	if ranked >= 2 {
		return common.SimpleResponse(406, "That beatmap is already ranked.")
	}

	switch err {
	case nil:
		// move on
	case sql.ErrNoRows:
		data, _ := json.Marshal(d)
		md.R.Publish("lets:beatmap_updates", string(data))
	default:
		md.Err(err)
		return Err500
	}

	err = md.DB.QueryRow("SELECT 1 FROM map_requests WHERE map_id = ? AND time > ?",
		d.ID, time.Now().Add(-time.Hour*24).Unix()).Scan(new(int))

	// error handling
	switch err {
	case sql.ErrNoRows:
		break
	case nil:
		// we're returning a success because if the request was already sent in the past 24
		// hours, it's as if the user submitted it.
		return BeatmapRankRequestsStatusGET(md)
	default:
		md.Err(err)
		return Err500
	}

	_, err = md.DB.Exec(
		"INSERT INTO map_requests (map_id, player_id, datetime, active) VALUES (?, ?, ?, 1)",
		d.ID, md.ID(), time.Now())
	if err != nil {
		md.Err(err)
		return Err500
	}

	return BeatmapRankRequestsStatusGET(md)
}
