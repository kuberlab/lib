package main

import (
	"fmt"
	"github.com/kuberlab/lib/pkg/mlapp"
	//"regexp"
	"time"
	"net/url"
)

func main() {
	c := &mlapp.Config{}
	c.KubeInits(nil, nil, nil)
	fmt.Println(url.QueryEscape(time.Now().Format(time.RFC3339)))
}
