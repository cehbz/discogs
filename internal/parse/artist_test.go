package parse

import (
	"strings"
	"testing"
)

func TestParseArtists(t *testing.T) {
	const xml = `<artists>
<artist><id>1</id><name>Aphex Twin</name><realname>Richard James</realname>
  <profile>bio</profile><data_quality>Correct</data_quality>
  <namevariations><name>AFX</name><name>Polygon Window</name></namevariations>
  <urls><url>https://aphextwin.com</url></urls>
  <aliases><name id="2">AFX</name></aliases>
  <groups><name id="9">The Tuss</name></groups>
</artist>
<artist><id>5</id><name>Autechre</name>
  <members><id>6</id><name id="6">Sean Booth</name><id>7</id><name id="7">Rob Brown</name></members>
</artist>
</artists>`
	var got []*Artist
	if err := ParseArtists(strings.NewReader(xml), func(a *Artist) error { got = append(got, a); return nil }); err != nil {
		t.Fatalf("ParseArtists: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	a := got[0]
	if a.ID != 1 || a.Name != "Aphex Twin" || a.RealName != "Richard James" {
		t.Fatalf("core: %+v", a)
	}
	if len(a.NameVariations) != 2 || a.NameVariations[1] != "Polygon Window" {
		t.Fatalf("namevars: %v", a.NameVariations)
	}
	if len(a.Aliases) != 1 || a.Aliases[0].ID != 2 {
		t.Fatalf("aliases: %+v", a.Aliases)
	}
	if len(a.Groups) != 1 || a.Groups[0].ID != 9 {
		t.Fatalf("groups: %+v", a.Groups)
	}
	if len(got[1].Members) != 2 || got[1].Members[0].ID != 6 || got[1].Members[1].Name != "Rob Brown" {
		t.Fatalf("members: %+v", got[1].Members)
	}
}
