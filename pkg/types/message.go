package types

type Message struct {
	ID      string      `json:"id"`
	Type    string      `json:"type"`
	Content interface{} `json:"content"`
}
