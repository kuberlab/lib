package errors

import (
	"github.com/jinzhu/gorm"
	"net/http"
)

const (
	MessageUnknownError = "unknown error"
)

type errorReason string

type Error struct {
	Status     int    `json:"status"`
	Message    string `json:"message,omitempty"`
	Reason     errorReason
	dbNotFound bool
}

func (e *Error) Error() string {
	if len(e.Message) == 0 {
		return MessageUnknownError
	}
	return e.Message
}

func (e *Error) HttpStatus() int {
	if e.Status <= 0 {
		return http.StatusInternalServerError
	}
	return e.Status
}

func New(text string) error {
	return NewStatus(http.StatusInternalServerError, text)
}

func NewStatus(status int, text string) error {
	return Smart(status, text)
}

func NewStatusReason(status int, text, reason string) error {
	return Smart(status, text, Reason(reason))
}

func Reason(reason string) errorReason {
	return errorReason(reason)
}

func Smart(args ...interface{}) error {
	err := &Error{}
	var statusSet, messageSet, reasonSet, errSet bool
	for _, arg := range args {
		switch a := arg.(type) {
		case *Error:
			if errSet {
				continue
			}
			if a.dbNotFound {
				err.Status = http.StatusNotFound
				err.dbNotFound = true
				statusSet = true
			} else if !statusSet {
				err.Status = a.Status
			}
			if !messageSet {
				err.Message = a.Message
			}
			if !reasonSet {
				err.Reason = a.Reason
				reasonSet = true
			}
			errSet = true
		case error:
			if errSet {
				continue
			}
			if a == gorm.ErrRecordNotFound {
				err.Status = http.StatusNotFound
				err.dbNotFound = true
				if !messageSet {
					err.Message = a.Error()
				}
			} else {
				if messageSet {
					continue
				}
				err.Message = a.Error()
				messageSet = true
			}
			errSet = true
		case errorReason:
			if reasonSet {
				continue
			}
			err.Reason = a
			reasonSet = true
		case string:
			if messageSet {
				continue
			}
			err.Message = a
			messageSet = true
		case int:
			if statusSet {
				continue
			}
			err.Status = a
			statusSet = true
		}
	}
	return err
}
