package naming

import "testing"

func TestFormat(t *testing.T) {
	inSeries := TokenData{
		AuthorName:     "Terry Pratchett",
		AuthorSortName: "Pratchett, Terry",
		BookTitle:      "The Colour of Magic",
		SeriesTitle:    "Discworld",
		SeriesPosition: 1,
		ReleaseYear:    "1983",
	}
	standalone := TokenData{
		AuthorName: "Neil Gaiman",
		BookTitle:  "Coraline",
	}

	cases := []struct {
		name     string
		template string
		data     TokenData
		want     string
	}{
		{"author folder", "{Author Name}", inSeries, "Terry Pratchett"},
		{"sort name", "{Author SortName}", inSeries, "Pratchett, Terry"},
		{"series file", "{Series Title} {Series Position} - {Book Title}", inSeries, "Discworld 1 - The Colour of Magic"},
		{"same template, no series", "{Series Title} {Series Position} - {Book Title}", standalone, "Coraline"},
		{"year suffix", "{Book Title} ({Release Year})", inSeries, "The Colour of Magic (1983)"},
		{"year suffix, unknown year", "{Book Title} ({Release Year})", standalone, "Coraline"},
		{"hash marker", "{Series Title} #{Series Position} - {Book Title}", inSeries, "Discworld #1 - The Colour of Magic"},
		{"hash marker, no series", "{Series Title} #{Series Position} - {Book Title}", standalone, "Coraline"},
		{"unknown token stays literal", "{Bogus} - {Book Title}", standalone, "{Bogus} - Coraline"},
	}
	for _, c := range cases {
		if got := Format(c.template, c.data); got != c.want {
			t.Errorf("%s: Format(%q) = %q, want %q", c.name, c.template, got, c.want)
		}
	}
}

func TestFormatFractionalPosition(t *testing.T) {
	d := TokenData{BookTitle: "The Sea and Little Fishes", SeriesTitle: "Discworld", SeriesPosition: 22.5}
	got := Format("{Series Title} {Series Position} - {Book Title}", d)
	if got != "Discworld 22.5 - The Sea and Little Fishes" {
		t.Errorf("got %q", got)
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"Guards! Guards!":       "Guards! Guards!",
		`What: "A" <Question>?`: "What A Question",
		"Trailing dots...":      "Trailing dots",
		"Sla/sh\\es":            "Slashes",
		"":                      "_",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}
