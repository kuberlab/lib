package types

type Message struct {
	ID      string      `json:"id"`
	Ts      int64       `json:"ts"`
	Type    string      `json:"type"`
	Content interface{} `json:"content"`
}
