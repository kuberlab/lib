package main

import (
	"fmt"
	//"github.com/kuberlab/lib/pkg/mlapp"
	//"regexp"
	"time"
	"net/url"
)

func main() {
	/*c := &mlapp.Config{}
	c.KubeInits(nil, nil, nil)
	var validNames *regexp.Regexp = regexp.MustCompile("^[a-z0-9][-a-z0-9]*[a-z0-9]$")
	fmt.Println("res:", validNames.MatchString("ps-A3"))
	charNotFitToKube := regexp.MustCompile("[^-a-z0-9_]")
	fmt.Println("res:", charNotFitToKube.ReplaceAllString("Aa_9s*", "-"))*/
	fmt.Println(url.QueryEscape(time.Now().Format(time.RFC3339)))
}
