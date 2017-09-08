package main

import (
	"fmt"
	"net/http"
)

func main() {
	resp, err := http.Get("https://dev.kuberlab.io")
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.StatusCode)
}
