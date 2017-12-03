package main

import (
	//"github.com/kuberlab/lib/pkg/mlapp"
	"regexp"
	"fmt"
)

func main() {
	//c := &mlapp.Config{}
	///c.KubeInits(nil, nil, nil)
	r := regexp.MustCompile("^[a-zA-Z][_\\-a-zA-Z0-9]+[a-zA-Z0-9]$")
	fmt.Println("res:",r.MatchString("jkA2-1"))
	charNotFitToKube := regexp.MustCompile("[^-a-z0-9_]")
	fmt.Println("res:",charNotFitToKube.ReplaceAllString("Aa_9s*","-"))
}
