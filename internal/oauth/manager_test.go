package oauth

import (
	"testing"
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

func TestManager_ShouldRefresh(t *testing.T) {
	manager := GetManager()

	// Case 1: No API Key
	p1 := &model.OAuthProvider{APIKey: ""}
	if !manager.ShouldRefresh(p1) {
		t.Error("expected should refresh when no api key")
	}

	// Case 2: Expired
	p2 := &model.OAuthProvider{APIKey: "key", APIKeyExpireAt: time.Now().Unix() - 100}
	if !manager.ShouldRefresh(p2) {
		t.Error("expected should refresh when expired")
	}

	// Case 3: About to expire (within 1 hour)
	p3 := &model.OAuthProvider{APIKey: "key", APIKeyExpireAt: time.Now().Unix() + 1800} // 30 mins
	if !manager.ShouldRefresh(p3) {
		t.Error("expected should refresh when about to expire")
	}

	// Case 4: Valid
	p4 := &model.OAuthProvider{APIKey: "key", APIKeyExpireAt: time.Now().Unix() + 7200} // 2 hours
	if manager.ShouldRefresh(p4) {
		t.Error("expected shouldn't refresh when valid")
	}
}

func TestGetActiveAuthJson(t *testing.T) {
	// Case 1: No auth_jsons
	p1 := &model.OAuthProvider{AuthJsons: []model.AuthJson{}}
	if aj := p1.GetActiveAuthJson(); aj != nil {
		t.Error("expected nil for no auth_jsons")
	}

	// Case 2: All disabled
	p2 := &model.OAuthProvider{
		AuthJsons: []model.AuthJson{
			{Content: `{"RefreshToken":"test"}`, Enabled: false},
		},
	}
	if aj := p2.GetActiveAuthJson(); aj != nil {
		t.Error("expected nil for all disabled")
	}

	// Case 3: StatusCode 200 preferred
	p3 := &model.OAuthProvider{
		AuthJsons: []model.AuthJson{
			{Content: `{"RefreshToken":"test1"}`, Enabled: true, StatusCode: 200, LastUseTimeStamp: 1000},
			{Content: `{"RefreshToken":"test2"}`, Enabled: true, StatusCode: 0, LastUseTimeStamp: 2000},
		},
	}
	aj3 := p3.GetActiveAuthJson()
	if aj3 == nil || aj3.GetRefreshToken() != "test1" {
		t.Error("expected test1 for StatusCode 200 preferred")
	}

	// Case 4: Most recent LastUseTimeStamp among StatusCode 200
	p4 := &model.OAuthProvider{
		AuthJsons: []model.AuthJson{
			{Content: `{"RefreshToken":"test1"}`, Enabled: true, StatusCode: 200, LastUseTimeStamp: 1000},
			{Content: `{"RefreshToken":"test2"}`, Enabled: true, StatusCode: 200, LastUseTimeStamp: 2000},
		},
	}
	aj4 := p4.GetActiveAuthJson()
	if aj4 == nil || aj4.GetRefreshToken() != "test2" {
		t.Error("expected test2 for most recent LastUseTimeStamp")
	}

	// Case 5: Empty content skipped
	p5 := &model.OAuthProvider{
		AuthJsons: []model.AuthJson{
			{Content: "", Enabled: true},
			{Content: `{"RefreshToken":"test"}`, Enabled: true, StatusCode: 0},
		},
	}
	aj5 := p5.GetActiveAuthJson()
	if aj5 == nil || aj5.GetRefreshToken() != "test" {
		t.Error("expected test for non-empty content")
	}
}
