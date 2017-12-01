package main

import (
	//"github.com/kuberlab/lib/pkg/utils"
	//"fmt"
	//"net/url"
	"k8s.io/apimachinery/pkg/api/resource"
	//"net/url"
	"fmt"
	//"github.com/kuberlab/lib/pkg/utils"
)

func main() {
	//fmt.Println(utils.KubeEncode("gSj_s+0-priner213.5"))
	//fmt.Println(url.PathEscape("dsd/dsd"))
	q,err := resource.ParseQuantity("0")
	if err!=nil{
		panic(err)
	}
	q1,err := resource.ParseQuantity("1Ki")
	if err!=nil{
		panic(err)
	}
	fmt.Println(q.String())
	fmt.Println(q1.AsInt64())
	fmt.Println(q1.Cmp(q))
}
