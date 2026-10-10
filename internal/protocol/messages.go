package protocol

type Hello struct {
	Type            string `json:"type"`
	BinSlug         string `json:"binSlug"`
	ServerVersion   string `json:"serverVersion"`
	ProtocolVersion int    `json:"protocolVersion"`
	ExpiresAt       string `json:"expiresAt"`
	Permanent       bool   `json:"permanent"`
}

type Request struct {
	Type       string              `json:"type"`
	AttemptID  string              `json:"attemptId"`
	RequestID  string              `json:"requestId"`
	Replay     bool                `json:"replay"`
	Method     string              `json:"method"`
	Path       string              `json:"path"`
	Query      string              `json:"query"`
	Headers    map[string][]string `json:"headers"`
	BodyBase64 string              `json:"bodyBase64"`
}

type Response struct {
	Type       string              `json:"type"`
	AttemptID  string              `json:"attemptId"`
	RequestID  string              `json:"requestId"`
	Status     int                 `json:"status"`
	Headers    map[string][]string `json:"headers"`
	BodyBase64 string              `json:"bodyBase64"`
	DurationMs int64               `json:"durationMs"`
}

type Error struct {
	Type       string `json:"type"`
	AttemptID  string `json:"attemptId"`
	RequestID  string `json:"requestId"`
	Message    string `json:"message"`
	DurationMs int64  `json:"durationMs"`
}

type Ready struct {
	Type       string `json:"type"`
	CliVersion string `json:"cliVersion"`
	TargetURL  string `json:"targetUrl"`
}
