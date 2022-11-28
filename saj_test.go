package saj

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReader_Error(t *testing.T) {
	data := []string{
		`"incomplete string`,
		`"not an escape \e"`,
		`"not a hex char \uMIDL"`,
		`undefined`,
		`falsy`,
		`truthy`,
		`["unclosed", "array`,
		`["trailing", "comma", ]`,
		`{"name" "foobar"}`,
		`{"name": "foobar",}`,
		`{"name": }`,
		`{"unclosed": "object"`,
		`{true: false}`,
	}
	for _, d := range data {
		r := New(strings.NewReader(d))
		e, err := r.Read()
		if err == nil {
			t.Errorf("%s: invalid json parsed properly as %v", d, e)
		}
	}
}

func TestReader(t *testing.T) {
	data := []struct {
		Input string
		Type  ElementType
	}{
		{
			Input: `"foobar"`,
			Type:  TypeString,
		},
		{
			Input: `   "foobar"    `,
			Type:  TypeString,
		},
		{
			Input: `"foo\"bar"`,
			Type:  TypeString,
		},
		{
			Input: `"foo\u00AFbar"`,
			Type:  TypeString,
		},
		{
			Input: `-3.14`,
			Type:  TypeNumber,
		},
		{
			Input: `0.14`,
			Type:  TypeNumber,
		},
		{
			Input: `42`,
			Type:  TypeNumber,
		},
		{
			Input: `true`,
			Type:  TypeBool,
		},
		{
			Input: `null`,
			Type:  TypeNull,
		},
		{
			Input: `[]`,
			Type:  TypeArray,
		},
		{
			Input: `[    ]`,
			Type:  TypeArray,
		},
		{
			Input: `[  3  , 10, 20  ]`,
			Type:  TypeArray,
		},
		{
			Input: `[true, false, null, "string", 3e+18]`,
			Type:  TypeArray,
		},
		{
			Input: `{}`,
			Type:  TypeObject,
		},
		{
			Input: `{    }`,
			Type:  TypeObject,
		},
		{
			Input: `{  "name"  : "foobar" ,  "age":  10  }`,
			Type:  TypeObject,
		},
		{
			Input: `{"name": "foobar", "age": 10, "enabled": false}`,
			Type:  TypeObject,
		},
		{
			Input: `[{"name": "foo"}, {"name": "bar"}]`,
			Type:  TypeArray,
		},
	}
	for _, d := range data {
		r := New(strings.NewReader(d.Input))
		e, err := r.Read()
		if err != nil && !errors.Is(err, io.EOF) {
			t.Errorf("%s: unexpected error: %s", d.Input, err)
			continue
		}
		if e == nil {
			t.Errorf("%s: nil element received (%s)", d.Input, err)
			continue
		}
		if e.Type() != d.Type {
			t.Errorf("%s: unexpected element type", d.Input)
			continue
		}
	}
}
