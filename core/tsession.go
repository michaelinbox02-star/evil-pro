package core

type TSession struct {
	ID         int                    `json:"id"`
	Phishlet   string                 `json:"phishlet"`
	LandingURL string                 `json:"landing_url"`
	Username   string                 `json:"username"`
	Password   string                 `json:"password"`
	Custom     map[string]interface{} `json:"custom"`
	BodyTokens map[string]interface{} `json:"body_tokens"`
	HTTPTokens map[string]interface{} `json:"http_tokens"`
	Tokens     map[string]interface{} `json:"tokens"`
	SessionID  string                 `json:"session_id"`
	UserAgent  string                 `json:"useragent"`
	RemoteAddr string                 `json:"remote_addr"`
	CreateTime int64                  `json:"create_time"`
	UpdateTime int64                  `json:"update_time"`
}
