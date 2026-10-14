package terminal

// EngineFact is one line /mcp shows under an engine: an i18n key for the
// label and the value. ValueKey, when set, is the value's own i18n key (a
// selection reason); NoteKey is translated after the value.
type EngineFact struct{ Key, Value, ValueKey, NoteKey string }
