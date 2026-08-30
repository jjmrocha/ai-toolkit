package packs

// ToolPack owns the tools one call registered in a ToolBox, and whatever serves
// them.
type ToolPack interface {
	// Close stops the server and removes the tools it registered. Nothing else
	// owns the server, so a ToolPack dropped rather than closed leaves it
	// running for the life of the program. It is safe to call more than once.
	Close() error
}
