package subagent

// Config describes one kind of sub-agent: what it is called, what it is for,
// and how its conversations begin. Pass it to [Manager.Register].
type Config struct {
	// Name identifies the agent in [Manager.Run]. Required.
	Name string
	// Description tells the model choosing a sub-agent what this one is for. See
	// [Manager.Agents].
	Description string
	// Prompt becomes the child's system message.
	Prompt string
}

// Info is what [Manager.Agents] reports about a registered agent.
type Info struct {
	// Name is the agent's Config.Name.
	Name string
	// Description is the agent's Config.Description.
	Description string
}
