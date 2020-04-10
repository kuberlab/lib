package types

import (
	"fmt"
	"testing"
)

func TestTimeMilli(t *testing.T) {
	s := "2020-02-10T10:35:30.075Z"
	tt := MustParseMilli(s)

	if tt.String() != "2020-02-10T10:35:30.075Z" {
		t.Fatal("must be 2020-02-10T10:35:30.075Z, but got ", tt.String())
	}

	tt = TimeMilli{}
	err := json.Unmarshal([]byte(fmt.Sprintf("%q", s)), &tt)
	if err != nil {
		t.Fatal(err)
	}
	if tt.String() != "2020-02-10T10:35:30.075Z" {
		t.Fatal("must be 2020-02-10T10:35:30.075Z, but got ", tt.String())
	}

	bts, err := json.Marshal(tt)
	if err != nil {
		t.Fatal(err)
	}
	if string(bts) != `"2020-02-10T10:35:30.075Z"` {
		t.Fatal("must be 2020-02-10T10:35:30.075Z, but got ", string(bts))
	}
}
