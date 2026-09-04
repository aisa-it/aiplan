package deferred

import "time"

type deadlinePayload struct {
	Body     string    `json:"body"`
	Deadline time.Time `json:"deadline"`
}

type messagePayload struct {
	Title string `json:"title"`
	Msg   string `json:"msg"`
}

type workspaceMessagePayload struct {
	Title     string   `json:"title"`
	Msg       string   `json:"msg"`
	AuthorID  string   `json:"author_id"`
	MemberIDs []string `json:"member_ids"`
}

type serviceMessagePayload struct {
	Title   string   `json:"title"`
	Msg     string   `json:"msg"`
	UserIDs []string `json:"user_ids"`
}
