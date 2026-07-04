package errs

import (
	"encoding/json"
	"fmt"
)

// renderTable maps every class to its model-visible phrasing (3.9). This
// is the NORMALIZED form; per-provider wire shapes map in S4.7. The model
// gets actionable guidance, not stack traces.
var renderTable = map[Class]string{
	ProviderRateLimit: "the model service is rate limited; the harness already retried",
	ProviderServer:    "the model service had a server error; the harness already retried",
	ProviderAuth:      "the model service rejected the harness credentials",
	ProviderInvalid:   "the request was rejected as invalid",
	ToolFailed:        "the tool failed",
	Timeout:           "the operation timed out",
	Canceled:          "the operation was canceled",
	Internal:          "an internal harness error occurred",
}

// RenderForModel produces the uniform model-visible error result for a
// classified failure. Deterministic — safe to use inside the fold.
func RenderForModel(class Class, detail string) json.RawMessage {
	phrase, ok := renderTable[class]
	if !ok {
		phrase = renderTable[Internal]
	}
	msg := phrase
	if detail != "" {
		msg = fmt.Sprintf("%s: %s", phrase, detail)
	}
	raw, _ := json.Marshal(map[string]string{"error": msg, "class": string(class)})
	return raw
}
