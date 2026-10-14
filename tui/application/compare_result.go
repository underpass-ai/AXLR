package application

// CompareResult is kmp_relate's reading, compact: the relations declared
// between the abouts, the proposals KMP makes from shared coordinates, and
// how many entries it read per about.
type CompareResult struct {
	Abouts    []string
	Declared  []MemoryRelation
	Proposals []MemoryRelation
	Facts     map[string]int
}
