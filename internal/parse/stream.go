package parse

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
)

// ArtistRef is an id+name reference to another artist (aliases, members, groups).
type ArtistRef struct {
	ID   int    `xml:"id,attr"`
	Name string `xml:",chardata"`
}

// LabelRef is an id+name reference to another label (parentLabel, sublabels).
type LabelRef struct {
	ID   int    `xml:"id,attr"`
	Name string `xml:",chardata"`
}

// ArtistCredit is a credited artist on a master, release, or track.
type ArtistCredit struct {
	ID     int    `xml:"id"`
	Name   string `xml:"name"`
	ANV    string `xml:"anv"`
	Join   string `xml:"join"`
	Role   string `xml:"role"`
	Tracks string `xml:"tracks"`
}

// Video is a linked video on a master or release.
type Video struct {
	URI         string `xml:"src,attr"`
	Duration    int    `xml:"duration,attr"`
	Title       string `xml:"title"`
	Description string `xml:"description"`
}

// streamRecords streams top-level <local> elements from r, decoding each into a
// fresh T and invoking fn. It never holds more than one record in memory.
func streamRecords[T any](r io.Reader, local string, fn func(*T) error) error {
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != local {
			continue
		}
		var rec T
		if err := dec.DecodeElement(&rec, &se); err != nil {
			return fmt.Errorf("decode <%s>: %w", local, err)
		}
		if err := fn(&rec); err != nil {
			return err
		}
	}
}
