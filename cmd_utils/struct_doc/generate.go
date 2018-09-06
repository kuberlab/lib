// @APIVersion 1.0.0
// @Title Documentation Generator
// @Description  Documentation Generator
// @Contact support@kuberlab.com
package main

import (
	//"github.com/gorilla/mux"
	//"log"
	"net/http"
	//"github.com/kuberlab/lib/pkg/mlapp"
	"github.com/kuberlab/lib/pkg/mlapp"
)

type MLAppModel struct {
	*mlapp.Config
}

// @Title ping
// @Description Gets run-time information about this service.
// @Accept  json
// @Success 200 {object} MLAppModel
// @Router /mlappconfig [get]
func mlAppConfig(w http.ResponseWriter, r *http.Request) {}

func main() {
	//r := mux.NewRouter()
	//r.Methods("GET").Path("/mlappconfig").HandlerFunc(mlAppConfig)
	//log.Fatal(http.ListenAndServe(":8088", r))
}