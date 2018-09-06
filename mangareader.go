package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

type resource struct {
	url  *url.URL
	info Metadata
}

type MangaReaderScraper struct{}

func mapSelectionText(i int, s *goquery.Selection) string {
	return s.Text()
}

func (m MangaReaderScraper) GetChapters(doc *goquery.Document) (chapters []resource) {
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
		name := match[2]

		chapterinfo := Metadata{
			"chapterIndex": i + 1,
			"chapter":      num,
			"chapterName":  name,
			"date":         s.Next().Text(),
		}
		chapterinfo.Update(mangainfo)

		u, err := doc.Url.Parse(link)
		if err != nil {
			log.Fatalln("cannot extract chapters:", err)
		}
		chapters = append(chapters, resource{u, chapterinfo})
	})

	if len(chapters) < 1 {
		log.Fatal("cannot extract chapters: none found")
	}
	return
}

func (m MangaReaderScraper) GetPages(doc *goquery.Document) (pages []resource, images []resource) {
	options := doc.Find("#pageMenu option")
	options.Each(func(i int, s *goquery.Selection) {
		value, ok := s.Attr("value")
		if !ok {
			log.Fatal("cannot extract pages: no link")
		}

		info := Metadata{
			"pages": options.Length(),
			"page":  i + 1,
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
			pages = append(pages, resource{u, info})
		}
	})

	return
}

func (m MangaReaderScraper) GetImage(doc *goquery.Document) resource {
	imgSrc, ok := doc.Find("#img").Attr("src")
	if !ok {
		log.Fatal("cannot extract image: no #img or @src")
	}

	imgURL, err := url.Parse(imgSrc)
	if err != nil {
		log.Fatalln("cannot extract image:", err)
	}
	return resource{imgURL, Metadata{"image_ext": "jpg"}} // XXX: are all images jpgs
}

type MangaReaderCrawler struct {
	shouldGuess bool
	scraper     MangaReaderScraper
	client      Fetcher
	saver       Saver
	rule        Rule
	observers   []Observer
}

var (
	IMAGE_NAME_RE = regexp.MustCompile(`(?P<prefix>.*)-(?P<number>\d+).(?P<suffix>.*)`)
)

func (m *MangaReaderCrawler) onPageEnd(page resource) {
	for _, o := range m.observers {
		o.OnPageEnd(page.info)
	}
}

func (m *MangaReaderCrawler) onChapterEnd(chapter resource) {
	for _, o := range m.observers {
		o.OnChapterEnd(chapter.info)
	}
}

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
//   pages: a list of page resources
//   images: a list of image resources
// Returns:
//   a list of (hopefuly correct) image resources
//
// Actually, one filename is not enough.  The general format of an image URL
// from mangareader.net is:
//     http://{host}/{chapterpath}/{manganame}-{number}.{extension}
// where the numbers always increase monotonically.  They are not however
// consecutive, though their difference remains the same within a single
// chapter.  To guess them then, requires that another image be downloaded.
func (m *MangaReaderCrawler) guessImages(ctx context.Context, pages []resource, images []resource) (pagesRem []resource, guesses []*url.URL) {
	if len(images) == 0 {
		log.Fatal("cannot guess images: no images given")
	}
	if len(pages) == 0 {
		// wow, single page chapter
		return
	}

	thisImageRes := images[0]
	lastImageRes, err := m.handlePage(ctx, pages[len(pages)-1])
	if err != nil {
		log.Fatal(err)
	}
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

func (m *MangaReaderCrawler) handleManga(ctx context.Context, mangaURL *url.URL) {
	mangaDoc, err := m.client.GetHTML(ctx, mangaURL)
	if err != nil {
		if err.(*url.Error).Err == context.Canceled {
			return
		}
		log.Fatal("handleManga", err)
	}

	wg := sync.WaitGroup{}
	chapters := m.scraper.GetChapters(mangaDoc)
	for _, c := range chapters {
		wg.Add(1)
		go func(c resource) {
			defer wg.Done()
			m.handleChapter(ctx, c)
		}(c)
	}
	wg.Wait()
}

func (m *MangaReaderCrawler) handleChapter(ctx context.Context, chapter resource) {
	chapterDoc, err := m.client.GetHTML(ctx, chapter.url)
	if err != nil {
		if err.(*url.Error).Err == context.Canceled {
			return
		}
		log.Fatal(err)
	}

	if chapter.info == nil {
		pathParts := strings.Split(chapter.url.EscapedPath(), "/")

		chapterPath := chapter.url.EscapedPath()
		mangaURL, _ := chapter.url.Parse(path.Dir(chapterPath))
		if len(pathParts) == 3 {
			// chapter url (/one-piece/2)
		} else if len(pathParts) == 4 {
			// chapter url with ending slash (/one-piece/2/)
			// or page url (/one-piece/2/3)
			chapterPath = path.Dir(chapterPath)
			mangaURL, _ = chapter.url.Parse(path.Dir(chapterPath))
		}

		mangaDoc, err := m.client.GetHTML(ctx, mangaURL)
		if err != nil {
			if err.(*url.Error).Err == context.Canceled {
				return
			}
			log.Fatal(err)
		}
		allChapters := m.scraper.GetChapters(mangaDoc)

		for _, c := range allChapters {
			if chapterPath == c.url.EscapedPath() {
				chapter.info = c.info
				break
			}
		}
	}

	otherPages, thisPage := m.scraper.GetPages(chapterDoc)
	thisPage[0].info.Update(chapter.info)
	for i := 0; i < len(otherPages); i++ {
		otherPages[i].info.Update(chapter.info)
	}

	if m.rule.Block(thisPage[0].info) {
		return
	}

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		m.handleImage(ctx, thisPage[0])
	}()

	if !m.shouldGuess {
		for _, p := range otherPages {
			wg.Add(1)
			go func(p resource) {
				defer wg.Done()
				// TODO: handle errors
				m.handlePage(ctx, p)
			}(p)
		}

	} else {
		pages, guesses := m.guessImages(ctx, otherPages, thisPage)
		for i, p := range pages {
			wg.Add(1)
			go func(page resource, imgURL *url.URL) {
				defer wg.Done()
				img := resource{imgURL, Metadata{"image_ext": "jpg"}} // XXX: are all images jpgs
				img.info.Update(page.info)
				err := m.handleImage(ctx, img)

				if err == nil {
					m.onPageEnd(img)
				} else {
					// XXX: don't inspect error messages
					if strings.HasPrefix(err.Error(), "GET") && strings.HasSuffix(err.Error(), "404") {
						m.handlePage(ctx, page)
					} else {
						log.Fatal(err)
					}
				}
			}(p, guesses[i])
		}
	}

	wg.Wait()
	m.onPageEnd(thisPage[0])
	m.onChapterEnd(thisPage[0])
}

func (m *MangaReaderCrawler) handlePage(ctx context.Context, page resource) (resource, error) {
	pageDoc, err := m.client.GetHTML(ctx, page.url)
	if err != nil {
		return resource{}, err
	}
	img := m.scraper.GetImage(pageDoc)
	img.info.Update(page.info)

	if err := m.handleImage(ctx, img); err != nil {
		return resource{}, err
	}

	m.onPageEnd(img)
	return img, nil
}

func (m *MangaReaderCrawler) handleImage(ctx context.Context, img resource) error {
	r, err := m.client.Get(ctx, img.url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	out, err := m.saver.Save(img.info, r.ContentLength)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := Copy(ctx, out, r.Body); err != nil {
		return err
	}
	return nil
}

type chapterObserverFunc func(Metadata)

func (fn chapterObserverFunc) OnPageEnd(info Metadata) {}
func (fn chapterObserverFunc) OnChapterEnd(info Metadata) {
	fn(info)
}

func NewMangaReaderCrawler(fetcher Fetcher, saver Saver, rule Rule) *MangaReaderCrawler {
	crawler := &MangaReaderCrawler{
		shouldGuess: false,
		client:      fetcher,
		saver:       saver,
		rule:        rule,
	}

	// for debugging
	crawler.Subscribe(chapterObserverFunc(func(info Metadata) {
		log.Println("finished chapter", info["chapterIndex"])
	}))

	return crawler
}

func (m *MangaReaderCrawler) Handle(ctx context.Context, u *url.URL) {
	pathParts := strings.Split(u.EscapedPath(), "/")
	if len(pathParts) == 2 {
		m.handleManga(ctx, u)
	} else if len(pathParts) == 4 || len(pathParts) == 3 && pathParts[len(pathParts)-1] != "" {
		m.handleChapter(ctx, resource{u, nil})
	} else {
		log.Fatalln("mangareader: cannot handle", u)
	}
}

func (m *MangaReaderCrawler) Subscribe(o Observer) {
	m.observers = append(m.observers, o)
}
