package packs

// ToolPack owns the tools one call registered in a ToolBox, and whatever serves
// them.
type ToolPack interface {
	// Close removes the tools the pack registered and stops whatever serves
	// them. A pack served by a process of its own leaves that process running
	// for the life of the program when it is dropped rather than closed, since
	// nothing else owns it. It is safe to call more than once.
	Close() error
}
