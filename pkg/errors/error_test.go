package errors

import (
	"net/http"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestErrorNew(t *testing.T) {

	err1 := New("")
	isEqual(MessageUnknownError, err1.Error(), t)
	err1e := err1.(*Error)
	isEqual(http.StatusInternalServerError, err1e.HttpStatus(), t)

	err2 := New("new err")
	isEqual("new err", err2.Error(), t)

}

func TestErrorNewStatus(t *testing.T) {

	err1 := NewStatus(http.StatusBadRequest, "")
	isEqual(MessageUnknownError, err1.Error(), t)
	err1e := err1.(*Error)
	isEqual(http.StatusBadRequest, err1e.HttpStatus(), t)

	err2 := NewStatus(http.StatusBadGateway, "new err")
	isEqual("new err", err2.Error(), t)
	err2e := err2.(*Error)
	isEqual(http.StatusBadGateway, err2e.HttpStatus(), t)

}

func TestErrorSmart(t *testing.T) {

	err1 := Smart()
	isEqual(MessageUnknownError, err1.Error(), t)
	err1e := err1.(*Error)
	isEqual(http.StatusInternalServerError, err1e.HttpStatus(), t)

	err2 := Smart(http.StatusBadRequest)
	isEqual(MessageUnknownError, err2.Error(), t)
	err2e := err2.(*Error)
	isEqual(http.StatusBadRequest, err2e.HttpStatus(), t)

	err3 := Smart("new err")
	isEqual("new err", err3.Error(), t)
	err3e := err3.(*Error)
	isEqual(http.StatusInternalServerError, err3e.HttpStatus(), t)

	err4 := Smart(http.StatusBadRequest, "bad req err")
	isEqual("bad req err", err4.Error(), t)
	err4e := err4.(*Error)
	isEqual(http.StatusBadRequest, err4e.HttpStatus(), t)

	err5 := Smart(http.StatusNotFound, "not found err", http.StatusBadRequest, "bad req err")
	isEqual("not found err", err5.Error(), t)
	err5e := err5.(*Error)
	isEqual(http.StatusNotFound, err5e.HttpStatus(), t)

	err6 := Smart(err4, err5)
	isEqual("bad req err", err6.Error(), t)
	err6e := err6.(*Error)
	isEqual(http.StatusBadRequest, err6e.HttpStatus(), t)

	err7 := Smart(err4, "another err")
	isEqual("another err", err7.Error(), t)
	err7e := err7.(*Error)
	isEqual(err4e.HttpStatus(), err7e.HttpStatus(), t)

	err8 := Smart("another err", err4)
	isEqual("another err", err8.Error(), t)
	err8e := err8.(*Error)
	isEqual(err4e.HttpStatus(), err8e.HttpStatus(), t)

	err9 := Smart(err4, http.StatusBadGateway)
	isEqual(err4.Error(), err9.Error(), t)
	err9e := err9.(*Error)
	isEqual(http.StatusBadGateway, err9e.HttpStatus(), t)

	err10 := Smart(http.StatusBadGateway, err4)
	isEqual(err4.Error(), err10.Error(), t)
	err10e := err10.(*Error)
	isEqual(http.StatusBadGateway, err10e.HttpStatus(), t)

	errDbNotFound := &Error{Message: "db not found", Status: http.StatusNotFound, dbNotFound: true}

	err11 := Smart(http.StatusBadRequest, errDbNotFound)
	isEqual(errDbNotFound.Error(), err11.Error(), t)
	err11e := err11.(*Error)
	isEqual(http.StatusNotFound, err11e.HttpStatus(), t)

	err12 := Smart(errDbNotFound, http.StatusBadRequest)
	isEqual(errDbNotFound.Error(), err12.Error(), t)
	err12e := err12.(*Error)
	isEqual(http.StatusNotFound, err12e.HttpStatus(), t)

}

func isEqual(want, got interface{}, t *testing.T) {
	if reflect.DeepEqual(want, got) {
		return
	}
	_, file, line, _ := runtime.Caller(1)
	splitted := strings.Split(file, string(os.PathSeparator))
	t.Fatalf("%v:%v: Failed: got %v, want %v", splitted[len(splitted)-1], line, got, want)
}
