package quality

// BaseQuery asks for the source of a language version as it was when
// the version was last current with it: the source's blob SourceBlob
// when a mark on the version applies (content.TranslationMark), else the
// source as it was at the newest commit that changed the version.
// Translation and Source are repository paths.
type BaseQuery struct {
	Translation string
	Source      string
	SourceBlob  string
}

// SourceBase answers one BaseQuery.
type SourceBase struct {
	// Known is false when the base can't be read: the version is undated,
	// older than the history read, or the source was not there at the
	// base. Such a version is not judged, never guessed.
	Known bool
	// Same says the source's file is now what it was at the base.
	Same bool
	// Content is the source's file at the base, when it is not the same.
	Content []byte
}

// BaseReader answers the queries of one run, in order: one SourceBase
// per query. It is called at most once per run, with at most
// MaxBaseQueries queries. An error with answers keeps the answers read
// before it; an error without any skips the rule.
type BaseReader func(queries []BaseQuery) ([]SourceBase, error)
