package models

type Prompt struct {
	To    string `json:"to"`
	Cron  string `json:"cron,omitempty"`
	Body  string `json:"body"`
}

type Receipt struct {
	To      string `json:"to"`
	Status  string `json:"status"`
	Time    int64  `json:"time"`
}
