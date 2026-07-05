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
			// The #8 series marker parses as Volume; ebook scoring ignores it.
			Parsed{Title: "Guards! Guards!", Formats: []string{"epub", "mobi", "azw3"}, Volume: 8},
		},
		{
			"Der Mort (German) PDF",
			Parsed{Title: "Der Mort", Formats: []string{"pdf"}, Language: "german"},
		},
		{
			"Some Linux ISO x264-GRP",
			Parsed{Title: "Some Linux ISO x264-GRP"},
		},
		{
			"Terry Pratchett - Mort (Unabridged) M4B 64kbps read by Nigel Planer",
			Parsed{Author: "Terry Pratchett", Title: "Mort", Formats: []string{"m4b"},
				Bitrate: 64, Narrator: "Nigel Planer"},
		},
		{
			"Mort [Abridged] [MP3 128k] narrated by Tony Robinson",
			Parsed{Title: "Mort", Formats: []string{"mp3"}, Bitrate: 128,
				Abridged: true, Narrator: "Tony Robinson"},
		},
		{
			"Berserk v05 (2021) (Digital) [CBZ]",
			Parsed{Title: "Berserk", Year: 2021, Formats: []string{"cbz"}, Volume: 5},
		},
		{
			"The Walking Dead #12 CBR",
			Parsed{Title: "The Walking Dead", Formats: []string{"cbr"}, Volume: 12},
		},
		{
			"Berserk Volume 41 (Dark Horse) cbz",
			Parsed{Title: "Berserk", Formats: []string{"cbz"}, Volume: 41},
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

func TestPreferencesFromProfile(t *testing.T) {
	prefs := PreferencesFromProfile(library.QualityProfile{
		Formats:     []string{"azw3", "epub"},
		Language:    "german",
		RetailBonus: 40,
		MinSize:     100,
		MaxSize:     1000,
	})
	if prefs.FormatScores["azw3"] != 100 || prefs.FormatScores["epub"] != 80 {
		t.Errorf("format scores = %v", prefs.FormatScores)
	}
	if _, ok := prefs.FormatScores["pdf"]; ok {
		t.Error("unlisted format should be absent (rejected)")
	}

	// An epub-only German profile rejects English pdf, prefers azw3.
	pdf := Score(rel("Mort PDF", indexer.ProtocolUsenet, 500, -1), prefs, nil, nil)
	if pdf.Approved {
		t.Errorf("pdf approved under azw3/epub profile: %+v", pdf)
	}
	azw3 := Score(rel("Der Mort AZW3 German Retail", indexer.ProtocolUsenet, 500, -1), prefs, nil, nil)
	if !azw3.Approved || azw3.Score != 150 { // 100 + retail 40 + usenet 10
		t.Errorf("azw3 = %+v, want approved score 150", azw3)
	}

	// Long format lists floor at 20.
	many := PreferencesFromProfile(library.QualityProfile{
		Formats: []string{"epub", "azw3", "mobi", "pdf", "cbz", "cbr"},
	})
	if many.FormatScores["cbz"] != 20 || many.FormatScores["cbr"] != 20 {
		t.Errorf("floored scores = %v", many.FormatScores)
	}
}

func TestScoreAudiobook(t *testing.T) {
	prefs := DefaultAudiobookPreferences()

	m4b := Score(rel("Mort Unabridged M4B", indexer.ProtocolUsenet, 200<<20, -1), prefs, nil, nil)
	if !m4b.Approved {
		t.Fatalf("m4b rejected: %v", m4b.Rejections)
	}
	mp3 := Score(rel("Mort MP3 64kbps", indexer.ProtocolUsenet, 200<<20, -1), prefs, nil, nil)
	if !mp3.Approved || mp3.Score >= m4b.Score {
		t.Errorf("mp3 should approve below m4b: %+v vs %+v", mp3, m4b)
	}
	abridged := Score(rel("Mort Abridged M4B", indexer.ProtocolUsenet, 200<<20, -1), prefs, nil, nil)
	if abridged.Approved {
		t.Error("abridged should be rejected for audiobooks")
	}
	epub := Score(rel("Mort EPUB", indexer.ProtocolUsenet, 200<<20, -1), prefs, nil, nil)
	if epub.Approved {
		t.Error("ebook format should be rejected under audiobook prefs")
	}
	// Ebook-sized files are suspicious for audio.
	tiny := Score(rel("Mort M4B", indexer.ProtocolUsenet, 1<<20, -1), prefs, nil, nil)
	if tiny.Approved {
		t.Error("1 MiB audiobook should be rejected")
	}
}

func TestScoreVolume(t *testing.T) {
	prefs := DefaultMangaPreferences()

	right := ScoreVolume(rel("Berserk v05 (Digital) CBZ", indexer.ProtocolUsenet, 50<<20, -1), prefs, "Berserk", 5)
	if !right.Approved {
		t.Fatalf("right volume rejected: %v", right.Rejections)
	}
	wrongVol := ScoreVolume(rel("Berserk v06 CBZ", indexer.ProtocolUsenet, 50<<20, -1), prefs, "Berserk", 5)
	if wrongVol.Approved {
		t.Error("wrong volume approved")
	}
	noVol := ScoreVolume(rel("Berserk Complete CBZ", indexer.ProtocolUsenet, 50<<20, -1), prefs, "Berserk", 5)
	if noVol.Approved {
		t.Error("volume-less release approved")
	}
	wrongSeries := ScoreVolume(rel("One Piece v05 CBZ", indexer.ProtocolUsenet, 50<<20, -1), prefs, "Berserk", 5)
	if wrongSeries.Approved {
		t.Error("wrong series approved")
	}
	epubUnderComic := ScoreVolume(rel("Berserk v05 EPUB", indexer.ProtocolUsenet, 50<<20, -1), DefaultComicPreferences(), "Berserk", 5)
	if epubUnderComic.Approved {
		t.Error("epub approved under comic prefs")
	}
}

func TestScoreMagazine(t *testing.T) {
	prefs := DefaultMagazinePreferences()
	owned := map[string]bool{"2026-06-27": true}

	fresh, id := ScoreMagazine(rel("The Economist - 2026-07-04 PDF", indexer.ProtocolUsenet, 50<<20, -1), prefs, "The Economist", owned)
	if !fresh.Approved || id != "2026-07-04" {
		t.Fatalf("fresh issue = %+v (id %q)", fresh.Rejections, id)
	}

	ownedIssue, _ := ScoreMagazine(rel("The Economist - 2026-06-27 PDF", indexer.ProtocolUsenet, 50<<20, -1), prefs, "The Economist", owned)
	if ownedIssue.Approved {
		t.Error("owned issue should be rejected")
	}

	wrongMag, _ := ScoreMagazine(rel("Wired - 2026-07 PDF", indexer.ProtocolUsenet, 50<<20, -1), prefs, "The Economist", owned)
	if wrongMag.Approved {
		t.Error("different magazine should be rejected")
	}

	noIssue, _ := ScoreMagazine(rel("The Economist PDF", indexer.ProtocolUsenet, 50<<20, -1), prefs, "The Economist", owned)
	if noIssue.Approved {
		t.Error("release without issue identifier should be rejected")
	}

	numbered, id := ScoreMagazine(rel("Retro Gamer Issue 261 PDF", indexer.ProtocolUsenet, 50<<20, -1), prefs, "Retro Gamer", nil)
	if !numbered.Approved || id != "issue-261" {
		t.Errorf("numbered issue = %+v (id %q)", numbered.Rejections, id)
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
