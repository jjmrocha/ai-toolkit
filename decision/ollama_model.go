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
	Type string  `json:"type"`
	Noul float64 `json:"noul"`
}

type ollamaUsage struct {
	InputTokens int `json:"input_tokens"`
}
