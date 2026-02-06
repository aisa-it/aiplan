package email

type DigestView struct {
	Title  string
	IsNew  bool
	IsGone bool
}

type TransitionFlags struct {
	Added   bool
	Removed bool
}

type FieldPrerender struct {
	Verb  string
	Value string
	Count int
}
