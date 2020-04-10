package types

import (
	jsonStd "encoding/json"
	"reflect"
	"strconv"
	"time"

	"github.com/kuberlab/lib/pkg/utils"
)

const (
	FormatMilli = "2006-01-02T15:04:05.000Z"
)

type TimeMilli struct {
	Time
}

func ParseTimeMilli(s string) (time.Time, error) {
	t, err := time.ParseInLocation(FormatMilli, s, time.FixedZone("UTC", 0))
	if err != nil {
		return time.ParseInLocation(OldFormat, s, time.FixedZone("UTC", 0))
	}
	return t, err
}

func NewTimeMilli(t time.Time) TimeMilli {
	return TimeMilli{Time: Time{Time: t.UTC().Truncate(time.Millisecond), Valid: true}}
}

func NewTimeMilliPtr(t time.Time) *TimeMilli {
	tt := NewTimeMilli(t)
	return &tt
}

func TimeMilliNow() TimeMilli {
	return NewTimeMilli(time.Now())
}

func MustParseMilli(s string) TimeMilli {
	return NewTimeMilli(utils.MustParse(s))
}

func (t TimeMilli) String() string {
	return t.Time.Time.UTC().Format(FormatMilli)
}

func (t TimeMilli) MarshalJSON() ([]byte, error) {
	if !t.Valid {
		return []byte("null"), nil
	}
	return []byte(t.Time.Time.UTC().Format(strconv.Quote(FormatMilli))), nil
}

func (t *TimeMilli) UnmarshalJSON(data []byte) error {
	var err error
	var v interface{}
	if err = json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch v := v.(type) {
	case string:
		tt, err := ParseTimeMilli(v)
		if err == nil {
			*t = TimeMilli{Time: Time{Time: tt, Valid: true}}
			return nil
		}
	case nil:
		*t = TimeMilli{Time: Time{Valid: false}}
		return nil
	}
	return &jsonStd.UnmarshalTypeError{Value: "time", Type: reflect.TypeOf(v)}
}
