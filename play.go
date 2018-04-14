package main

import (
	"fmt"
	"net/url"
	"time"
	"bytes"
	"text/template"
)

type Env struct {
	Name            string `json:"name"`
	Value           string `json:"value"`
	ValueFromSecret string `json:"valueFromSecret"`
	SecretKey       string `json:"secretKey"`
}
func main() {
	fmt.Println(url.QueryEscape(time.Now().Format(time.RFC3339)))
	envs := []Env{
		{
			Name:"jk",
			Value:"{{ .rt }}-test",
		},
		{
			Name:"rt",
			Value:"1",
		},
	}
	ResolveEnv(envs)
	for _,e := range envs{
		println(e.Name,e.Value)
	}
}

func ResolveEnv(envs []Env){
	vars := map[string]string{}
	for _,e := range envs{
		if e.SecretKey=="" {
			vars[e.Name] = e.Value
		}
	}
	for i,e := range envs{
		if e.SecretKey != ""{
			continue
		}
		t := template.New("gotpl")
		if t, err := t.Parse(e.Value);err==nil{
			buffer := bytes.NewBuffer(make([]byte, 0))
			if err := t.ExecuteTemplate(buffer, "gotpl", vars); err == nil {
				envs[i].Value = buffer.String()
			}
		}
	}
}