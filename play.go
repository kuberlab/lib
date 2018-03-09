package main

import (
	"fmt"
	"net/url"
	"time"
)

func main() {
	fmt.Println(url.QueryEscape(time.Now().Format(time.RFC3339)))
}
