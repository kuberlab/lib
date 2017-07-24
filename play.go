package main

import (
	"fmt"
	"github.com/kuberlab/lib/pkg/example"
	"github.com/kuberlab/lib/pkg/mlapp"
)

func main() {
	c, err := mlapp.NewConfig([]byte(example.TF_EXAMPLE), mlapp.BuildOption("ws-id", "ws-name", "my-app"))
	if err != nil {
		panic(err)
	}
	fmt.Println(*c)
	_, err = c.GenerateTaskResources()
	if err != nil {
		panic(err)
	}
	_, err = c.GenerateUIXResources()
	if err != nil {
		panic(err)
	}
}
