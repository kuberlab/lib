package main

import (
	"github.com/kuberlab/lib/pkg/mlapp"
)

func main() {
	c := &mlapp.Config{}
	c.KubeInits(nil, nil, nil)
}
