package iflow

type IFlowTokenStorage struct {
	APIKey      string `json:"api_key"`
	Email       string `json:"email"`
	Expire      string `json:"expire"` // Format: "2006-01-02 15:04"
	Cookie      string `json:"cookie"`
	LastRefresh string `json:"last_refresh"`
}

type IFlowAPIKeyResponse struct {
	Success bool `json:"success"`
	Data    struct {
		APIKey     string `json:"apiKey"`
		ExpireTime string `json:"expireTime"`
		HasExpired bool   `json:"hasExpired"`
	} `json:"data"`
}
