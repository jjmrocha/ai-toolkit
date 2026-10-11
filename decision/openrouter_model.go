package decision

type orDecisionRequest struct {
	Model     string                `json:"model"`
	State     string                `json:"state"`
	Questions map[string]orQuestion `json:"questions"`
}

type orQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type orDecisionResponse struct {
	Answers map[string]orAnswer `json:"answers"`
	Usage   orUsage             `json:"usage"`
	Error   *orError            `json:"error"`
}

type orAnswer struct {
	Type          string             `json:"type"`
	Noul          float64            `json:"noul"`
	Choice        string             `json:"choice"`
	Score         float64            `json:"score"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
	Legend        map[string]string  `json:"legend"`
}

type orUsage struct {
	InputTokens int `json:"input_tokens"`
}

type orError struct {
	Message string `json:"message"`
}
