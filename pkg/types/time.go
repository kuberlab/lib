package types

import (
	"database/sql/driver"
	"encoding/json"
	"reflect"
	"strconv"
	"time"

	"github.com/kuberlab/lib/pkg/utils"
)

const (
	Format    = "2006-01-02T15:04:05"
	SQLLayout = "2006-01-02 15:04:05"
)

type Time struct {
	Time  time.Time
	Valid bool
}

func NewTime(t time.Time) Time {
	return Time{Time: t.UTC().Truncate(time.Second), Valid: true}
}

func NewTimePtr(t time.Time) *Time {
	tt := NewTime(t)
	return &tt
}

func TimeNow() Time {
	return NewTime(time.Now())
}

func MustParse(s string) Time {
	return NewTime(utils.MustParse(s))
}

func ParseTime(s string) (time.Time, error) {
	return time.ParseInLocation(Format, s, time.FixedZone("UTC", 0))
}

func (t *Time) Scan(v interface{}) error {
	t.Time, t.Valid = v.(time.Time)
	if t.Valid {
		// Time from DB may come with zone included.
		// Convert time to UTC.
		_, offset := t.Time.Zone()
		t.Time = t.Time.Add(time.Second * time.Duration(offset))
		t.Time = t.Time.UTC()
	}
	return nil
}

func (t Time) Value() (driver.Value, error) {
	if !t.Valid {
		return nil, nil
	}
	return t.Time.UTC().Format(SQLLayout), nil
}

func (t Time) MarshalJSON() ([]byte, error) {
	if !t.Valid {
		return []byte("null"), nil
	}
	return []byte(time.Time(t.Time.UTC()).Format(strconv.Quote(Format))), nil
}

func (t *Time) UnmarshalJSON(data []byte) error {
	var err error
	var v interface{}
	if err = json.Unmarshal(data, &v); err != nil {
		return err
	}
	switch v := v.(type) {
	case string:
		tt, err := ParseTime(v)
		if err == nil {
			*t = Time{Time: tt, Valid: true}
			return nil
		}
	case nil:
		*t = Time{Valid: false}
		return nil
	}
	return &json.UnmarshalTypeError{Value: "time", Type: reflect.TypeOf(v)}
}

func (t Time) String() string {
	return time.Time(t.Time.UTC()).Format(SQLLayout)
}

func (t Time) SQLFormat() string {
	return t.String()
}

func (t Time) ServiceFormat() string {
	return time.Time(t.Time.UTC()).Format(Format)
}

func (t Time) Before(u Time) bool {
	return (t.Time.UTC()).Before(u.Time.UTC())
}

func (t Time) After(u Time) bool {
	return (t.Time.UTC()).After(u.Time.UTC())
}

func (t Time) Add(u time.Duration) Time {
	return NewTime(t.Time.Add(u))
}

func (t Time) Equal(u Time) bool {
	return t.Time.Equal(u.Time)
}
