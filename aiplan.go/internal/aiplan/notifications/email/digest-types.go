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

type fieldPrerender struct {
	Value string
	Count int
}
