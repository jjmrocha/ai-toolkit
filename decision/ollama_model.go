package decision

type ollamaDecisionRequest struct {
	Model     string                    `json:"model"`
	State     string                    `json:"state"`
	Questions map[string]ollamaQuestion `json:"questions"`
}

type ollamaQuestion struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

type ollamaDecisionResponse struct {
	Answers map[string]ollamaAnswer `json:"answers"`
	Usage   ollamaUsage             `json:"usage"`
}

type ollamaAnswer struct {
	Type          string             `json:"type"`
	Noul          float64            `json:"noul"`
	Choice        string             `json:"choice"`
	Score         float64            `json:"score"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

type ollamaUsage struct {
	InputTokens int `json:"input_tokens"`
}
