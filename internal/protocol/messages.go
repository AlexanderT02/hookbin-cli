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

// Ready is sent after hello and again after every target switch. Features lists optional abilities
// such as "retarget"; servers that do not know a feature ignore it.
type Ready struct {
	Type       string   `json:"type"`
	CliVersion string   `json:"cliVersion"`
	TargetURL  string   `json:"targetUrl"`
	Features   []string `json:"features,omitempty"`
}

// Target asks the CLI to forward to another local URL from now on.
type Target struct {
	Type      string `json:"type"`
	TargetURL string `json:"targetUrl"`
}

// Features this CLI announces in ready.
var Features = []string{"retarget"}
