package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/server/middleware"
	"github.com/bestruirui/octopus/internal/server/resp"
	"github.com/bestruirui/octopus/internal/server/router"
	"github.com/bestruirui/octopus/internal/task"
	"github.com/dlclark/regexp2"
	"github.com/gin-gonic/gin"
)

// GroupListItemSpeed contains speed-test data for a group item.
type GroupListItemSpeed struct {
	ResponseTimeMs int    `json:"response_time_ms"`
	Status         string `json:"status"`
	LastError      string `json:"last_error,omitempty"`
}

// GroupItemWithSpeed extends GroupItem with speed-test data.
type GroupItemWithSpeed struct {
	model.GroupItem
	Speed *GroupListItemSpeed `json:"speed,omitempty"`
}

// GroupWithSpeed is the group list response payload including per-item speed data.
type GroupWithSpeed struct {
	ID                int                  `json:"id"`
	Name              string               `json:"name"`
	Mode              model.GroupMode      `json:"mode"`
	MatchRegex        string               `json:"match_regex"`
	FirstTokenTimeOut int                  `json:"first_token_time_out"`
	SessionKeepTime   int                  `json:"session_keep_time"`
	Items             []GroupItemWithSpeed `json:"items,omitempty"`
}

func init() {
	router.NewGroupRouter("/api/v1/group").
		Use(middleware.Auth()).
		Use(middleware.RequireJSON()).
		AddRoute(
			router.NewRoute("/list", http.MethodGet).
				Handle(getGroupList),
		).
		AddRoute(
			router.NewRoute("/create", http.MethodPost).
				Handle(createGroup),
		).
		AddRoute(
			router.NewRoute("/update", http.MethodPost).
				Handle(updateGroup),
		).
		AddRoute(
			router.NewRoute("/delete/:id", http.MethodDelete).
				Handle(deleteGroup),
		).
		AddRoute(
			router.NewRoute("/speed-test/:id", http.MethodPost).
				Handle(triggerGroupSpeedTest),
		)
	// AddRoute(
	// 	router.NewRoute("/auto-add-item", http.MethodPost).
	// 		Handle(autoAddGroupItem),
	// )
}

func getGroupList(c *gin.Context) {
	groups, err := op.GroupList(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Load all speed data
	speedMap, err := op.GetGroupChannelModelSpeedMapAll(c.Request.Context())
	if err != nil {
		// Log but don't fail - speed data is optional
		speedMap = make(map[string]*model.GroupChannelModelSpeed)
	}

	// Build response with speed data
	result := make([]GroupWithSpeed, 0, len(groups))
	for _, group := range groups {
		gws := GroupWithSpeed{
			ID:                group.ID,
			Name:              group.Name,
			Mode:              group.Mode,
			MatchRegex:        group.MatchRegex,
			FirstTokenTimeOut: group.FirstTokenTimeOut,
			SessionKeepTime:   group.SessionKeepTime,
			Items:             make([]GroupItemWithSpeed, 0, len(group.Items)),
		}

		for _, item := range group.Items {
			itemWithSpeed := GroupItemWithSpeed{
				GroupItem: item,
			}

			// Look up speed data for this item
			key := fmt.Sprintf("%d:%d:%s", group.ID, item.ChannelID, item.ModelName)
			if speed, ok := speedMap[key]; ok {
				itemWithSpeed.Speed = &GroupListItemSpeed{
					ResponseTimeMs: speed.ResponseTimeMs,
					Status:         speed.Status,
					LastError:      speed.LastError,
				}
			}

			gws.Items = append(gws.Items, itemWithSpeed)
		}

		result = append(result, gws)
	}

	resp.Success(c, result)
}

func createGroup(c *gin.Context) {
	var group model.Group
	if err := c.ShouldBindJSON(&group); err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if group.MatchRegex != "" {
		_, err := regexp2.Compile(group.MatchRegex, regexp2.ECMAScript)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := op.GroupCreate(&group, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, group)
}

func updateGroup(c *gin.Context) {
	var req model.GroupUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if req.MatchRegex != nil {
		_, err := regexp2.Compile(*req.MatchRegex, regexp2.ECMAScript)
		if err != nil {
			resp.Error(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	group, err := op.GroupUpdate(&req, c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, group)
}

func deleteGroup(c *gin.Context) {
	id := c.Param("id")
	idNum, err := strconv.Atoi(id)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := op.GroupDel(idNum, c.Request.Context()); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	resp.Success(c, "group deleted successfully")
}

func triggerGroupSpeedTest(c *gin.Context) {
	id := c.Param("id")
	idNum, err := strconv.Atoi(id)
	if err != nil {
		resp.Error(c, http.StatusBadRequest, err.Error())
		return
	}

	// Execute speed test and return after latest results are persisted.
	if err := task.RunGroupSpeedTestManual(c.Request.Context(), idNum); err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	resp.Success(c, gin.H{
		"message":  "speed test completed",
		"group_id": idNum,
	})
}

// func autoAddGroupItem(c *gin.Context) {
// 	var req struct {
// 		ID int `json:"id"`
// 	}
// 	if err := c.ShouldBindJSON(&req); err != nil {
// 		resp.Error(c, http.StatusBadRequest, err.Error())
// 		return
// 	}
// 	if req.ID <= 0 {
// 		resp.Error(c, http.StatusBadRequest, "invalid id")
// 		return
// 	}
// 	err := worker.AutoAddGroupItem(req.ID, c.Request.Context())
// 	if err != nil {
// 		resp.Error(c, http.StatusInternalServerError, err.Error())
// 		return
// 	}
// 	resp.Success(c, nil)
// }
