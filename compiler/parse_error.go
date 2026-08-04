package compiler

import (
	"fmt"

	"github.com/pablo-botella/cargoxml"
)

// ParseError describes a config parsing problem with enough context to point
// at the culprit: which element, which attribute (when attribute-level) and
// what is wrong. Line and Column locate it in the input when known.
type ParseError struct {
	Element string // element where the problem was found
	Attr    string // offending attribute, empty for element-level problems
	Msg     string // what is wrong
	Line    int    // 1-based line in the input, 0 when unknown
	Column  int    // 1-based column in the input, 0 when unknown
	Err     error  // wrapped cause, nil when the message says it all
}

// NewParseError creates a ParseError taking the context the decoder already
// knows: the innermost open element and the input position. The remaining
// fields (Attr, Err) are filled by the caller when they apply.
func NewParseError(d *cargoxml.DecoderWithCargo, msg string) *ParseError {
	e := &ParseError{Msg: msg}
	if d != nil {
		if frame := d.Stack.Current(); frame != nil && frame.NodeName != nil {
			e.Element = frame.NodeName.Local
		}
		if d.Decoder != nil {
			e.Line, e.Column = d.Decoder.InputPos()
		}
	}
	return e
}

func (e *ParseError) Error() string {
	s := "mkskill config"
	if e.Element != "" {
		s += ": <" + e.Element + ">"
	}
	if e.Attr != "" {
		s += " " + e.Attr
	}
	if e.Msg != "" {
		s += ": " + e.Msg
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	if e.Line > 0 {
		s += fmt.Sprintf(" (line %d:%d)", e.Line, e.Column)
	}
	return s
}

// Unwrap exposes the cause to errors.Is / errors.As.
func (e *ParseError) Unwrap() error {
	return e.Err
}
