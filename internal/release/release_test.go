package release

import (
	"reflect"
	"testing"

	"github.com/librinode/librinode/internal/indexer"
	"github.com/librinode/librinode/internal/library"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Parsed
	}{
		{
			"Terry Pratchett - Mort (1987) Retail EPUB",
			Parsed{Author: "Terry Pratchett", Title: "Mort", Year: 1987, Formats: []string{"epub"}, Retail: true},
		},
		{
			"Terry Pratchett - Discworld 04 - Mort [EPUB] [ENG]",
			Parsed{Author: "Terry Pratchett", Title: "Mort", Formats: []string{"epub"}, Language: "english"},
		},
		{
			"Mort by Terry Pratchett EPUB",
			Parsed{Author: "Terry Pratchett", Title: "Mort", Formats: []string{"epub"}},
		},
		{
			"Terry.Pratchett.-.Mort.1987.Retail.EPUB-GROUP",
			Parsed{Author: "Terry Pratchett", Title: "Mort", Year: 1987, Formats: []string{"epub"}, Retail: true, Group: "GROUP"},
		},
		{
			"Guards! Guards! (Discworld #8) [epub/mobi/azw3]",
			Parsed{Title: "Guards! Guards!", Formats: []string{"epub", "mobi", "azw3"}},
		},
		{
			"Der Mort (German) PDF",
			Parsed{Title: "Der Mort", Formats: []string{"pdf"}, Language: "german"},
		},
		{
			"Some Linux ISO x264-GRP",
			Parsed{Title: "Some Linux ISO x264-GRP"},
		},
	}
	for _, c := range cases {
		got := Parse(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q)\n got %+v\nwant %+v", c.in, got, c.want)
		}
	}
}

func rel(title string, protocol string, size int64, seeders int) indexer.Release {
	return indexer.Release{
		Indexer: "mock", Protocol: protocol, Title: title,
		Size: size, Seeders: seeders, Peers: seeders,
	}
}

func TestScoreGeneric(t *testing.T) {
	prefs := DefaultEbookPreferences()

	epub := Score(rel("Mort Retail EPUB", indexer.ProtocolUsenet, 1<<20, -1), prefs, nil, nil)
	if !epub.Approved {
		t.Fatalf("epub rejected: %v", epub.Rejections)
	}
	// epub 100 + retail 25 + usenet 10
	if epub.Score != 135 {
		t.Errorf("epub score = %d, want 135", epub.Score)
	}

	pdf := Score(rel("Mort PDF", indexer.ProtocolUsenet, 1<<20, -1), prefs, nil, nil)
	if !pdf.Approved || pdf.Score >= epub.Score {
		t.Errorf("pdf should approve but rank below epub: %+v", pdf)
	}

	noFormat := Score(rel("Mort", indexer.ProtocolUsenet, 1<<20, -1), prefs, nil, nil)
	if noFormat.Approved {
		t.Error("release without a format should be rejected")
	}

	dead := Score(rel("Mort EPUB", indexer.ProtocolTorrent, 1<<20, 0), prefs, nil, nil)
	if dead.Approved {
		t.Error("torrent with 0 seeders should be rejected")
	}

	seeded := Score(rel("Mort EPUB", indexer.ProtocolTorrent, 1<<20, 50), prefs, nil, nil)
	if !seeded.Approved || seeded.Score != 120 { // 100 + capped 20
		t.Errorf("seeded torrent = %+v, want score 120", seeded)
	}

	tiny := Score(rel("Mort EPUB", indexer.ProtocolUsenet, 512, -1), prefs, nil, nil)
	if tiny.Approved {
		t.Error("tiny file should be rejected")
	}

	german := Score(rel("Mort EPUB German", indexer.ProtocolUsenet, 1<<20, -1), prefs, nil, nil)
	if german.Approved {
		t.Error("non-preferred language should be rejected")
	}
}

func TestScoreAgainstBook(t *testing.T) {
	prefs := DefaultEbookPreferences()
	book := &library.Book{Title: "The Colour of Magic", ReleaseDate: "1983-11-24"}
	author := &library.Author{Name: "Terry Pratchett"}

	right := Score(rel("Terry Pratchett - The Colour of Magic (1983) EPUB", indexer.ProtocolUsenet, 1<<20, -1), prefs, book, author)
	if !right.Approved {
		t.Fatalf("correct release rejected: %v", right.Rejections)
	}

	// Article-stripped title still matches.
	stripped := Score(rel("Terry Pratchett - Colour of Magic EPUB", indexer.ProtocolUsenet, 1<<20, -1), prefs, book, author)
	if !stripped.Approved {
		t.Errorf("article-stripped title rejected: %v", stripped.Rejections)
	}

	wrongBook := Score(rel("Terry Pratchett - Mort EPUB", indexer.ProtocolUsenet, 1<<20, -1), prefs, book, author)
	if wrongBook.Approved {
		t.Error("different book should be rejected")
	}

	wrongAuthor := Score(rel("Stephen King - The Colour of Magic EPUB", indexer.ProtocolUsenet, 1<<20, -1), prefs, book, author)
	if wrongAuthor.Approved {
		t.Error("missing author mention should be rejected")
	}

	// Year drift is a penalty, not a rejection.
	reprint := Score(rel("Terry Pratchett - The Colour of Magic (2009) EPUB", indexer.ProtocolUsenet, 1<<20, -1), prefs, book, author)
	if !reprint.Approved {
		t.Fatalf("reprint rejected: %v", reprint.Rejections)
	}
	if reprint.Score >= right.Score {
		t.Errorf("reprint (%d) should score below original-year release (%d)", reprint.Score, right.Score)
	}
}

func TestRank(t *testing.T) {
	prefs := DefaultEbookPreferences()
	candidates := []Candidate{
		Score(rel("Mort", indexer.ProtocolUsenet, 1<<20, -1), prefs, nil, nil),             // rejected
		Score(rel("Mort PDF", indexer.ProtocolUsenet, 1<<20, -1), prefs, nil, nil),         // low
		Score(rel("Mort Retail EPUB", indexer.ProtocolUsenet, 1<<20, -1), prefs, nil, nil), // high
		Score(rel("Mort EPUB", indexer.ProtocolTorrent, 1<<20, 5), prefs, nil, nil),        // mid
	}
	Rank(candidates)
	if !candidates[0].Approved || candidates[0].Score != 135 {
		t.Errorf("first = %+v", candidates[0])
	}
	if candidates[len(candidates)-1].Approved {
		t.Error("rejected candidate should sort last")
	}
	for i := 1; i < 3; i++ {
		if candidates[i-1].Score < candidates[i].Score {
			t.Errorf("approved candidates out of order at %d", i)
		}
	}
}
