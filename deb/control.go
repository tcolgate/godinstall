package deb

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/clearsign"
	"golang.org/x/crypto/openpgp/errors"
)

const rfc2822DateLayout = "Mon, 2 Jan 2006 15:04:05 -0700"

// ParseDate parses an rfc2822 date as used by debian
// control files
func ParseDate(s string) (time.Time, error) {
	return time.Parse(rfc2822DateLayout, s)
}

// FormatTime formats a go time.Time as per the debian standard
// usage (rfc2822)
func FormatTime(t time.Time) string {
	return t.Format(rfc2822DateLayout)
}

// ControlFile repsents a set of debian control data
// paragraphs
type ControlFile struct {
	Data              []*ControlParagraph
	Signed            bool     // Was there a signature on the package
	SignatureVerified bool     // Was the files signature assoicated with a key we know
	SignedBy          []string // Who signed the file
}

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

// ParseControl parses the contents of the reader as a debian
// control file. It does not collapse folding or multiple value fields
// and assumes this has already been done.
func ParseControl(rawin io.Reader, kr openpgp.EntityList) (ControlFile, error) {
	c := ControlFile{}
	c.Data = []*ControlParagraph{}

	r := c.parseControlSignature(rawin, kr)

	var newpara = MakeControlParagraph()
	c.Data = append(c.Data, &newpara)

	scanner := bufio.NewScanner(r)
	var currfield string

	for scanner.Scan() {
		currpara := c.Data[len(c.Data)-1]
		line := scanner.Text()
		switch {
		case line == "":
			{
				// Skip empty sections, seen in some debs
				if len(*currpara) != 0 {
					var newp = MakeControlParagraph()
					c.Data = append(c.Data, &newp)
					currfield = ""
				}
			}
		case line[0] == ' ', line[0] == '\t':
			{
				if currfield == "" {
					return ControlFile{}, fmt.Errorf("continuation field with no active field present")
				}
				currpara.AddValue(currfield, strings.TrimSpace(line))
			}
		default:
			{
				vs := strings.SplitN(line, ":", 2)
				if len(vs) != 2 {
					return ControlFile{}, fmt.Errorf("No field name found in non-continuation line")
				}
				currfield = vs[0]
				val := ""
				if len(vs) > 1 {
					val = strings.TrimSpace(vs[1])
				}
				currpara.AddValue(currfield, val)
			}
		}
	}

	return c, nil
}

// WriteControl writes the control paragraphs to the io.Writer, using the keys
// from start first, if present. Any keys from end are output last, if present,
// any remaining keys. Other keys are output in between, sorted by key
func WriteControl(out io.Writer, c ControlFile, start []string, end []string) {
	for p := range c.Data {
		fields := c.Data[p]
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
		if p < len(c.Data)-1 {
			out.Write([]byte("\n"))
		}
	}
}

// Check any clear signed signature
func (c *ControlFile) parseControlSignature(rawin io.Reader, kr openpgp.EntityList) io.Reader {
	var br io.Reader

	// This could be a signed file, we'll peek first, as if it is, we need to
	// read the whole thing into RAm
	in := bufio.NewReader(rawin)

	pgpHeader := "-----BEGIN PGP SIGNED MESSAGE----"

	first, _ := in.Peek(len(pgpHeader))
	if string(first) != pgpHeader {
		c.SignatureVerified = false
		c.Signed = false
		c.SignedBy = []string{}

		return in
	}

	b, _ := ioutil.ReadAll(in)

	msg, rest := clearsign.Decode(b)
	switch {
	case msg == nil && len(rest) > 0:
		{
			c.Signed = false
			return bytes.NewReader(rest)
		}
	case msg != nil && len(msg.Plaintext) > 0:
		{
			if len(rest) > 0 {
				log.Println("trailing content in signed control file will be ignored")
			}

			br = bytes.NewReader(msg.Bytes)

			if kr == nil {
				kr = openpgp.EntityList{}
			}

			signedBy, err := openpgp.CheckDetachedSignature(kr, br, msg.ArmoredSignature.Body)
			switch err {
			case nil:
				c.SignatureVerified = true
				c.Signed = true
				for s := range signedBy.Identities {
					c.SignedBy = append(c.SignedBy, s)
				}
			case errors.ErrUnknownIssuer:
				c.Signed = true
				c.SignatureVerified = false
			default:
				c.Signed = false
				c.SignatureVerified = false
			}

			return bytes.NewReader(msg.Bytes)
		}
	default:
	}
	return br
}
