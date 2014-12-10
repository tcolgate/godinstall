package main

import (
	"bufio"
	"io"
	"sort"
	"strings"
)

// ControlFile repsents a set of debian control data
// paragraphs
type ControlFile []*ControlParagraph

// ControlParagraph represents a set of key value mappings
// read from a debian control file
type ControlParagraph map[string][]*string

// MakeControlParagraph initialises a new control paragraph
func MakeControlParagraph() ControlParagraph {
	return make(ControlParagraph)
}

// GetValues returns the set of values associated with a key
// from a paragraph of control data
func (ctrl ControlParagraph) GetValues(item string) ([]*string, bool) {
	v, ok := ctrl[item]
	return v, ok
}

// GetValue returns the first value associated with a key in a control
// paragraph
func (ctrl ControlParagraph) GetValue(item string) (string, bool) {
	v, ok := ctrl.GetValues(item)
	return *v[0], ok
}

// SetValue sets a control paragraph item to the single value provided
func (ctrl ControlParagraph) SetValue(item string, val string) {
	ctrl[item] = []*string{&val}
	return
}

// AddValue adds an additional value to a paragraph item
func (ctrl ControlParagraph) AddValue(item string, val string) {
	field, ok := ctrl[item]
	if ok {
		ctrl[item] = append(field, &val)
	} else {
		ctrl[item] = []*string{&val}
	}
	return
}

// ParseDebianControl parses the contents of the reader as a debian
// control file. It does not collapse folding or multiple value fields
// and assumes this has already been done.
func ParseDebianControl(rawin io.Reader) (ControlFile, error) {
	var paras = make(ControlFile, 1)
	var newpara = MakeControlParagraph()
	paras[0] = &newpara
	scanner := bufio.NewScanner(rawin)
	var currfield string

	for scanner.Scan() {
		currpara := paras[len(paras)-1]
		line := scanner.Text()
		switch {
		case line == "":
			{
				var newp = MakeControlParagraph()
				paras = append(paras, &newp)
			}
		case line[0] == ' ', line[0] == '\t':
			{
				currpara.AddValue(currfield, strings.TrimSpace(line))
			}
		default:
			{
				vs := strings.SplitN(line, ":", 2)
				currfield = vs[0]
				val := ""
				if len(vs) > 1 {
					val = strings.TrimSpace(vs[1])
				}
				currpara.AddValue(currfield, val)
			}
		}
	}

	return paras, nil
}

// WriteDebianControl writes the control paragraphs to the io.Writer, using the keys
// from start first, if present. Any keys from end are output last, if present,
// any remaining keys. Other keys are output in between, sorted by key
func WriteDebianControl(out io.Writer, paragraphs ControlFile, start []string, end []string) {
	for p := range paragraphs {
		fields := paragraphs[p]
		orderedMap := make(map[string]bool, len(*fields))

		// We don't want to repeat fields used in the start and
		// end lists, so track which ones we will output there
		for f := range start {
			orderedMap[start[f]] = true
		}
		for f := range end {
			orderedMap[end[f]] = true
		}

		// Output first fields
		for i := range start {
			fieldName := start[i]
			lines, ok := fields.GetValues(fieldName)
			if ok {
				out.Write([]byte(fieldName + ": " + *lines[0] + "\n"))
				rest := lines[1:]
				for j := range rest {
					out.Write([]byte(" " + *rest[j] + "\n"))
				}
			}
		}

		// Collate the remaining fields
		var middle []string
		for fieldName := range *fields {
			if !orderedMap[fieldName] {
				middle = append(middle, fieldName)
			}
		}

		sort.Strings(middle)
		for i := range middle {
			fieldName := middle[i]
			lines, ok := fields.GetValues(fieldName)
			if ok {
				out.Write([]byte(fieldName + ": " + *lines[0] + "\n"))
				rest := lines[1:]
				for j := range rest {
					out.Write([]byte(" " + *rest[j] + "\n"))
				}
			}
		}

		// Output final fields
		for i := range end {
			fieldName := end[i]
			lines, ok := fields.GetValues(fieldName)
			if ok {
				out.Write([]byte(fieldName + ": " + *lines[0] + "\n"))
				rest := lines[1:]
				for j := range rest {
					out.Write([]byte(" " + *rest[j] + "\n"))
				}
			}
		}

		// If this isn't the last paragraph, output a newline
		if p < len(paragraphs)-1 {
			out.Write([]byte("\n"))
		}
	}
}
