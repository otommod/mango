package main

import (
	"fmt"
	"log"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type MangaReaderScraper struct{}

func mapSelectionText(i int, s *goquery.Selection) string {
	return s.Text()
}

func (m MangaReaderScraper) GetChapters(doc *goquery.Document) (chapters []Resource) {
	mangainfo := Metadata{
		"manga":            doc.Find(".aname").Text(),
		"author":           doc.Find("td:contains('Author:') ~ td").Text(),
		"artist":           doc.Find("td:contains('Artist:') ~ td").Text(),
		"status":           doc.Find("td:contains('Status:') ~ td").Text(),
		"readingDirection": doc.Find("td:contains('Reading Direction:') ~ td").Text(),
		"genres":           doc.Find(".genretags").Map(mapSelectionText),
		"description":      doc.Find("#readmangasum p").Text(),
		"coverImage":       doc.Find("#mangaimg img").AttrOr("src", ""),
	}

	mangaName := mangainfo["manga"].(string)
	if len(mangaName) < 1 {
		log.Fatal("cannot extract chapters: no manga name")
	}

	readingDirection := mangainfo["readingDirection"].(string)
	if strings.ToLower(readingDirection) == "right to left" {
		mangainfo["readingDirection"] = "rtl"
	} else {
		mangainfo["readingDirection"] = "ltr"
	}

	listings := doc.Find("#listing td:first-child")
	mangainfo["chapters"] = listings.Length()

	listings.Each(func(i int, s *goquery.Selection) {
		links := s.Find("a[href]")
		if links.Length() != 1 {
			log.Fatal("cannot extract chapters: no link")
		}
		link, ok := links.Attr("href")
		if !ok {
			log.Fatal("cannot extract chapters: no link")
		}

		re := regexp.MustCompile(regexp.QuoteMeta(mangaName) + ` (?P<num>\d+) : (?P<name>.*)`)
		// match := re.FindStringSubmatch(strings.TrimLeftFunc(s.Text(), unicode.IsSpace))
		match := re.FindStringSubmatch(s.Text())
		if len(match) < 1 {
			log.Fatal("cannot extract chapters: no number")
		}
		num, _ := strconv.Atoi(match[1])

		chapterinfo := Metadata{
			"chapterIndex": i + 1,
			"chapter":      num,
			"chapterName":  match[2],
			// "dateAdded":    s.Next().Text(),
		}
		chapterinfo.Update(mangainfo)

		u, err := doc.Url.Parse(link)
		if err != nil {
			log.Fatalln("cannot extract chapters:", err)
		}
		chapters = append(chapters, Resource{u, chapterinfo})
	})

	if len(chapters) < 1 {
		log.Fatal("cannot extract chapters: none found")
	}
	return
}

func (m MangaReaderScraper) GetPages(doc *goquery.Document) (pages []Resource, images []Resource) {
	options := doc.Find("#pageMenu option")
	options.Each(func(i int, s *goquery.Selection) {
		value, ok := s.Attr("value")
		if !ok {
			log.Fatal("cannot extract pages: no link")
		}

		info := Metadata{
			"pages":     options.Length(),
			"pageIndex": i + 1,
		}

		u, err := doc.Url.Parse(value)
		if err != nil {
			log.Fatalln("cannot extract pages:", err)
		}
		if _, selected := s.Attr("selected"); selected {
			img := m.GetImage(doc)
			img.info.Update(info)
			images = append(images, img)
		} else {
			pages = append(pages, Resource{u, info})
		}
	})
	return
}

func (m MangaReaderScraper) GetImage(doc *goquery.Document) Resource {
	imgSrc, ok := doc.Find("#img").Attr("src")
	if !ok {
		log.Fatal("cannot extract image: no #img or @src")
	}

	imgURL, err := url.Parse(imgSrc)
	if err != nil {
		log.Fatalln("cannot extract image:", err)
	}
	return Resource{imgURL, Metadata{"imageExtension": "jpg"}} // XXX: are all images jpgs
}

type MangaReaderCrawler struct {
	shouldGuess bool
	CommonSimpleCrawler
}

var (
	IMAGE_NAME_RE = regexp.MustCompile(`(?P<prefix>.*)-(?P<number>\d+).(?P<suffix>.*)`)
)

func (m *MangaReaderCrawler) parseImageNumber(u *url.URL) (number int, pathFmt string) {
	basename := path.Base(u.EscapedPath())

	match := IMAGE_NAME_RE.FindStringSubmatch(basename)
	if len(match) < 1 {
		log.Fatal("cannot guess images: cannot extract file id")
	}

	var err error
	if number, err = strconv.Atoi(match[2]); err != nil {
		log.Fatalln("cannot guess images:", err)
	}

	pathFmt = fmt.Sprintf("./%s-%%d.%s",
		strings.Replace(match[1], "%", "%%", -1), strings.Replace(match[3], "%", "%%", -1))
	return
}

// Given the filename of one image, tries to guess the rest.
//
// Args:
//   pages: a list of page Resources
//   images: a list of image Resources
// Returns:
//   a list of (hopefuly correct) image Resources
//
// Actually, one filename is not enough.  The general format of an image URL
// from mangareader.net is:
//     http://{host}/{chapterpath}/{manganame}-{number}.{extension}
// where the numbers always increase monotonically.  They are not however
// consecutive, though their difference remains the same within a single
// chapter.  To guess them then, requires that another image be downloaded.
func (m *MangaReaderCrawler) guessImages(pages []Resource, images []Resource) (pagesRem []Resource, guesses []*url.URL) {
	if len(images) == 0 {
		log.Fatal("cannot guess images: no images given")
	}
	if len(pages) == 0 {
		// wow, single page chapter
		return
	}

	thisImageRes := images[0]
	lastImageRes := m.handlePage(pages[len(pages)-1])
	pages = pages[:len(pages)-1]

	thisPage := thisImageRes.info["page"].(int)
	lastPage := lastImageRes.info["page"].(int)
	if thisPage > lastPage {
		// could happen if thisPage is actual last page of the chapter and
		// lastPage is just the last in our list
		thisImageRes, lastImageRes = lastImageRes, thisImageRes
		thisPage, lastPage = lastPage, thisPage
	}

	thisImage, relPathFmt := m.parseImageNumber(thisImageRes.url)
	lastImage, _ := m.parseImageNumber(lastImageRes.url)

	delta := (lastImage - thisImage) / (lastPage - thisPage)
	start := thisImage - thisPage*delta

	log.Printf("%s@%d this:%d last:%d delta:%d",
		thisImageRes.info["manga"], thisImageRes.info["chapter"],
		thisImage, lastImage, delta)

	for _, p := range pages {
		page := p.info["page"].(int)
		newPath := fmt.Sprintf(relPathFmt, start+delta*page)
		u, _ := lastImageRes.url.Parse(newPath)
		pagesRem = append(pagesRem, p)
		guesses = append(guesses, u)
	}
	return
}

func NewMangaReaderCrawler(fetcher Fetcher, saver Saver, rule Rule, obs Observer) *MangaReaderCrawler {
	crawler := &MangaReaderCrawler{
		false,
		CommonSimpleCrawler{
			scraper: MangaReaderScraper{},
			client:  fetcher,
			saver:   saver,
			rule:    rule,
			obs:     obs,
		},
	}

	return crawler
}

func (m *MangaReaderCrawler) Handle(u *url.URL) {
	cleanPath := strings.TrimRight(u.EscapedPath(), "/")

	mangaURL := u
	switch strings.Count(cleanPath, "/") {
	case 3:
		// page url (/one-piece/2/3)
		cleanPath = path.Dir(cleanPath)
		fallthrough
	case 2:
		// chapter url (/one-piece/2)
		chapterPath := cleanPath
		mangaURL, _ = u.Parse(path.Dir(chapterPath))

		// add a rule to only download the requested chapter
		whitelistRule := funcRule(func(r Resource) bool {
			cleanPath := strings.TrimRight(r.url.EscapedPath(), "/")
			return strings.Count(cleanPath, "/") == 2 && cleanPath != chapterPath
		})
		m.rule = AndRule{whitelistRule, m.rule}
		fallthrough
	case 1:
		// manga url (/one-piece)
		m.handleManga(mangaURL)

	default:
		log.Fatalln("mangareader: cannot handle", u)
	}
}
