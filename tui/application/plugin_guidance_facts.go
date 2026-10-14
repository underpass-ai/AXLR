package application

// PluginGuidanceFacts is what the engine adapter needs to know about the
// session to word its paragraph: whether the model records memory itself
// in this mode and whether it has axlr_remember for it.
type PluginGuidanceFacts struct {
	RecordsMemory bool
	Remembers     bool
}
