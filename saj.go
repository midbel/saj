package saj

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"
)

type ElementType int

const (
	TypeObject ElementType = -(1 + iota)
	TypeArray
	TypeNumber
	TypeString
	TypeBool
	TypeNull
)

type Element interface {
	Type() ElementType
}

type Primitive interface {
	float64 | bool | string | struct{}
}

type Literal[T Primitive] struct {
	Literal T
}

func String(str string) Literal[string] {
	return Literal[string]{
		Literal: str,
	}
}

func Number(str string) (Literal[float64], error) {
	v, err := strconv.ParseFloat(str, 64)
	lit := Literal[float64]{
		Literal: v,
	}
	return lit, err
}

func Bool(str string) (Literal[bool], error) {
	b, err := strconv.ParseBool(str)
	lit := Literal[bool]{
		Literal: b,
	}
	return lit, err
}

func Null() Literal[struct{}] {
	return Literal[struct{}]{}
}

func (i Literal[T]) Type() ElementType {
	switch any(i.Literal).(type) {
	case string:
		return TypeString
	case bool:
		return TypeBool
	case float64:
		return TypeNumber
	default:
		return TypeNull
	}
}

type Array []Element

func (_ Array) Type() ElementType {
	return TypeArray
}

type Object map[string]Element

func (_ Object) Type() ElementType {
	return TypeObject
}

var errEmpty = errors.New("empty")

type Reader struct {
	rs    *bufio.Reader
	buf   bytes.Buffer
	depth int
}

func New(r io.Reader) *Reader {
	rs := Reader{
		rs: bufio.NewReader(r),
	}
	rs.skipBlank()
	return &rs
}

func (r *Reader) Read() (Element, error) {
	return r.read()
}

func (r *Reader) read() (Element, error) {
	defer func() {
		r.buf.Reset()
		r.skipBlank()
	}()

	c, err := r.next()
	if err != nil {
		return nil, err
	}
	var el Element
	switch {
	case isString(c):
		el, err = r.literal()
	case isObject(c):
		el, err = r.object()
	case isArray(c):
		el, err = r.array()
	case isDigit(c) || isMinus(c):
		r.reset()
		el, err = r.number()
	case isIdent(c):
		r.reset()
		el, err = r.identifier()
	case isBlank(c):
		r.skipBlank()
		return r.read()
	default:
		err = fmt.Errorf("read: unexpected character %c", c)
	}
	return el, err
}

func (r *Reader) object() (Element, error) {
	r.enter()
	defer r.leave()

	obj := make(Object)
	for {
		key, err := r.key()
		if err != nil {
			if errors.Is(err, errEmpty) {
				break
			}
			return nil, err
		}
		val, err := r.read()
		if err != nil {
			return nil, err
		}
		obj[key] = val

		c, err := r.next()
		if err != nil {
			return nil, err
		}
		if c == rcurly {
			return obj, nil
		} else if c == comma {
			r.skipBlank()
			if c, err := r.next(); c == rcurly || err != nil {
				return nil, fmt.Errorf("object: unexpected ',' before '}'")
			}
			r.reset()
		} else if isBlank(c) {
			break
		} else {
			return nil, fmt.Errorf("object: unexpected character %c", c)
		}
	}
	r.skipBlank()
	if c, _ := r.next(); c != rcurly {
		return nil, fmt.Errorf("object: expected '}', got %c", c)
	}
	return obj, nil
}

func (r *Reader) key() (string, error) {
	defer r.buf.Reset()
	r.skipBlank()

	c, _ := r.next()
	switch c {
	case quote:
	case rcurly:
		r.reset()
		return "", errEmpty
	default:
		return "", fmt.Errorf("key: '\"' expected, got %c", c)
	}
	key, err := r.literal()
	if err != nil {
		return "", err
	}
	r.skipBlank()
	if c, _ = r.next(); c != colon {
		return "", fmt.Errorf("object: ':' expected, got %c", c)
	}
	r.skipBlank()
	if k, ok := key.(Literal[string]); ok {
		return k.Literal, nil
	}
	return "", fmt.Errorf("object: invalid key type")
}

func (r *Reader) array() (Element, error) {
	r.enter()
	defer r.leave()

	var arr Array
	for {
		r.skipBlank()
		if c, _ := r.next(); c == rsquare {
			return arr, nil
		} else {
			r.reset()
		}
		nod, err := r.read()
		if err != nil {
			return nil, err
		}
		arr = append(arr, nod)
		c, err := r.next()
		if err != nil {
			return nil, err
		}
		if c == rsquare {
			return arr, nil
		} else if c == comma {
			r.skipBlank()
			if c, err := r.next(); c == rsquare || err != nil {
				return nil, fmt.Errorf("array: unexpected ',' before ']'")
			}
			r.reset()
		} else if isBlank(c) {
			break
		} else {
			return nil, fmt.Errorf("array: unexpected character %c", c)
		}
	}
	r.skipBlank()
	if c, _ := r.next(); c != rsquare {
		return nil, fmt.Errorf("array: expected ']', got %c", c)
	}
	return arr, nil
}

func (r *Reader) number() (Element, error) {
	c, _ := r.next()
	if isSign(c) {
		r.buf.WriteRune(c)
		c, _ = r.next()
	}
	if c == '0' {
		r.buf.WriteRune(c)
		c, _ = r.next()
		if c == dot {
			err := r.fraction()
			if err != nil {
				return nil, err
			}
		} else if isDelimiter(c) {
			r.reset()
		} else {
			return nil, fmt.Errorf("unexpected character after 0, %c", c)
		}
		return Number(r.buf.String())
	}
	r.reset()

	var last rune
	for {
		c, err := r.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				last = utf8.RuneError
				break
			}
			return nil, err
		}
		if !isDigit(c) {
			last = c
			break
		}
		r.buf.WriteRune(c)
	}
	var err error
	switch last {
	case utf8.RuneError:
	case dot:
		err = r.fraction()
	case 'e', 'E':
		err = r.exponent(last)
	default:
		r.reset()
	}
	if err != nil {
		return nil, err
	}
	return Number(r.buf.String())
}

func (r *Reader) fraction() error {
	defer r.reset()
	r.buf.WriteRune(dot)
	for {
		c, err := r.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		if !isDigit(c) {
			break
		}
		r.buf.WriteRune(c)
	}
	return nil
}

func (r *Reader) exponent(exp rune) error {
	r.buf.WriteRune(exp)
	c, _ := r.next()
	switch {
	case isSign(c):
		r.buf.WriteRune(c)
	case isDigit(c):
		r.reset()
	default:
		return fmt.Errorf("number: unexpected character after exponent: %c", c)
	}
	defer r.reset()
	for {
		c, err := r.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
		if !isDigit(c) {
			break
		}
		r.buf.WriteRune(c)
	}
	return nil
}

func (r *Reader) literal() (Element, error) {
	for {
		c, err := r.next()
		if err != nil {
			return nil, err
		}
		if c == backslash {
			if err := r.escape(); err != nil {
				return nil, err
			}
			continue
		}
		if c == quote {
			break
		}
		r.buf.WriteRune(c)
	}
	return String(r.buf.String()), nil
}

func (r *Reader) escape() error {
	r.buf.WriteRune(backslash)
	c, _ := r.next()
	switch c {
	case 'b', 'f', 'n', 'r', 't', '/', quote, backslash:
		r.buf.WriteRune(c)
	case 'u':
		r.buf.WriteRune(c)
		for i := 0; i < 4; i++ {
			c, _ = r.next()
			if !isHex(c) {
				return fmt.Errorf("%c not a hex character", c)
			}
			r.buf.WriteRune(c)
		}
	default:
		return fmt.Errorf("unknown escape")
	}
	return nil
}

func (r *Reader) identifier() (Element, error) {
	defer r.reset()
	for {
		c, err := r.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		if isDelimiter(c) {
			break
		}
		r.buf.WriteRune(c)
	}
	switch ident := r.buf.String(); ident {
	case kwTrue, kwFalse:
		return Bool(ident)
	case kwNull:
		return Null(), nil
	default:
		return nil, fmt.Errorf("%s: identifier not recognized", ident)
	}
}

func (r *Reader) next() (rune, error) {
	c, _, err := r.rs.ReadRune()
	return c, err
}

func (r *Reader) reset() {
	r.rs.UnreadRune()
}

func (r *Reader) skipBlank() {
	defer r.reset()
	for {
		c, _ := r.next()
		if !isBlank(c) {
			break
		}
	}
}

func (r *Reader) enter() {
	r.depth++
}

func (r *Reader) leave() {
	r.depth--
}

const (
	kwNull  = "null"
	kwTrue  = "true"
	kwFalse = "false"
)

const (
	comma     = ','
	lcurly    = '{'
	rcurly    = '}'
	lsquare   = '['
	rsquare   = ']'
	nl        = '\n'
	cr        = '\r'
	quote     = '"'
	dot       = '.'
	colon     = ':'
	space     = ' '
	tab       = '\t'
	minus     = '-'
	plus      = '+'
	backslash = '\\'
)

func isDelimiter(r rune) bool {
	return isBlank(r) || r == comma || r == rsquare || r == rcurly
}

func isNL(r rune) bool {
	return r == nl || r == cr
}

func isSpace(r rune) bool {
	return r == space || r == tab
}

func isBlank(r rune) bool {
	return isNL(r) || isSpace(r)
}

func isObject(r rune) bool {
	return r == lcurly
}

func isArray(r rune) bool {
	return r == lsquare
}

func isString(r rune) bool {
	return r == quote
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isMinus(r rune) bool {
	return r == minus
}

func isSign(r rune) bool {
	return r == minus || r == plus
}

func isIdent(r rune) bool {
	return r == 't' || r == 'f' || r == 'n'
}

func isHex(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}
