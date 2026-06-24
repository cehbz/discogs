package parse

import (
	"strings"
	"testing"
)

func TestParseLabels(t *testing.T) {
	const xml = `<labels>
<label><id>1</id><name>Warp</name><contactinfo>info</contactinfo><profile>p</profile>
  <data_quality>Correct</data_quality>
  <urls><url>https://warp.net</url></urls>
  <parentLabel id="42">Parent Co</parentLabel>
  <sublabels><label id="7">Arancia</label><label id="8">Nuts</label></sublabels>
</label>
<label><id>42</id><name>Parent Co</name></label>
</labels>`
	var got []*Label
	err := ParseLabels(strings.NewReader(xml), func(l *Label) error { got = append(got, l); return nil })
	if err != nil {
		t.Fatalf("ParseLabels: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d labels, want 2", len(got))
	}
	w := got[0]
	if w.ID != 1 || w.Name != "Warp" || w.ContactInfo != "info" || w.DataQuality != "Correct" {
		t.Fatalf("bad core fields: %+v", w)
	}
	if len(w.URLs) != 1 || w.URLs[0] != "https://warp.net" {
		t.Fatalf("bad urls: %v", w.URLs)
	}
	if w.ParentLabel == nil || w.ParentLabel.ID != 42 {
		t.Fatalf("bad parent: %+v", w.ParentLabel)
	}
	if len(w.SubLabels) != 2 || w.SubLabels[0].ID != 7 || w.SubLabels[1].Name != "Nuts" {
		t.Fatalf("bad sublabels: %+v", w.SubLabels)
	}
}
