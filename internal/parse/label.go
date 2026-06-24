package parse

import "io"

type Label struct {
	ID          int        `xml:"id"`
	Name        string     `xml:"name"`
	ContactInfo string     `xml:"contactinfo"`
	Profile     string     `xml:"profile"`
	DataQuality string     `xml:"data_quality"`
	URLs        []string   `xml:"urls>url"`
	ParentLabel *LabelRef  `xml:"parentLabel"`
	SubLabels   []LabelRef `xml:"sublabels>label"`
}

func ParseLabels(r io.Reader, fn func(*Label) error) error {
	return streamRecords(r, "label", fn)
}
