package main

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type MangaEdenScrapper struct{}

func nextTextNode(s *goquery.Selection) *goquery.Selection {
	textNodes := []*html.Node{}
	for _, node := range s.Nodes {
		for ; node != nil; node = node.NextSibling {
			if node.Type == html.TextNode {
				textNodes = append(textNodes, node)
				break
			}
		}
	}

	return s.Slice(0, 0).AddNodes(textNodes...)
}

func (m *MangaEdenScrapper) GetChapters(doc *goquery.Document) (chapters []resource) {
	comicType := nextTextNode(doc.Find("#rightContent h4:contains('Type')")).Text()
	comicType = strings.ToLower(strings.TrimSpace(comicType))
	readingDirection := "ltr"
	if comicType == "japanese manga" || comicType == "chinese manhua" || comicType == "doujinshi" {
		readingDirection = "rtl"
	}

	status := nextTextNode(doc.Find("#rightContent h4:contains('Status')")).Text()
	status = strings.TrimSpace(status)

	mangainfo := Metadata{
		"manga":             doc.Find(".manga-title").Text(),
		"author":            doc.Find("#rightContent h4:contains('Author') + a").Text(),
		"artist":            doc.Find("#rightContent h4:contains('Artist') + a").Text(),
		"status":            status,
		"reading_direction": readingDirection,
		"genres":            doc.Find("#rightContent h4:contains('Genres') ~ a").Map(mapSelectionText),
		"description":       doc.Find("#mangaDescription").Text(),
		"cover_image":       doc.Find(".mangaImage2 img").AttrOr("src", ""),
	}

	mangaName := mangainfo["manga"].(string)
	if len(mangaName) < 1 {
		log.Fatal("cannot extract chapters: no manga name")
	}

	chapterLinks := doc.Find(".chapterLink")
	chaptersLen := len(strconv.Itoa(chapterLinks.Length()))

	chapterLinks.Each(func(i int, s *goquery.Selection) {
		if goquery.NodeName(s) != "a" {
			log.Fatal("cannot extract chapters: no link")
		}
		link, ok := s.Attr("href")
		if !ok {
			log.Fatal("cannot extract chapters: no link")
		}

		re := regexp.MustCompile(`(?P<num>\d+)(?:: (?P<name>.*))?`)
		// match := re.FindStringSubmatch(strings.TrimLeftFunc(s.Text(), unicode.IsSpace))
		match := re.FindStringSubmatch(s.Find("b").Text())
		if len(match) < 1 {
			log.Fatal("cannot extract chapters: no number")
		}
		num, _ := strconv.Atoi(match[1])
		name := match[2]

		chapterinfo := Metadata{
			"chapter":      num,
			"chapter_name": name,
			"chapters_len": chaptersLen,
			"date":         s.Parent().Parent().Find("td.chapterDate").Text(),
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

func (m *MangaEdenScrapper) GetPages(doc *goquery.Document) (pages []resource, images []resource) {
	options := doc.Find("#pageSelect option")
	pagesLen := len(strconv.Itoa(options.Length()))

	options.Each(func(i int, s *goquery.Selection) {
		value, ok := s.Attr("value")
		if !ok {
			log.Fatal("cannot extract pages: no link")
		}

		info := Metadata{
			"page":      i + 1,
			"pages_len": pagesLen,
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

func (m *MangaEdenScrapper) GetImage(page *goquery.Document) (img resource) {
	imgSrc, ok := page.Find("#mainImg").Attr("src")
	if !ok {
		log.Fatal("cannot extract image: no #img or @src")
	}

	imgURL, err := page.Url.Parse(imgSrc)
	if err != nil {
		log.Fatalln("cannot extract image:", err)
	}
	return resource{imgURL, Metadata{"image_ext": "jpg"}} // XXX: are all images jpgs
}

type MangaEdenCrawler struct {
	scraper MangaEdenScrapper
	client  Fetcher
	saver   Saver
	rule    Rule
	obs     Observer
}

func (m *MangaEdenCrawler) handleManga(mangaURL *url.URL) {
	mangaDoc, err := m.client.GetHTML(mangaURL)
	if err != nil {
		log.Fatal(err)
	}

	wg := sync.WaitGroup{}
	chapters := m.scraper.GetChapters(mangaDoc)
	for _, c := range chapters {
		wg.Add(1)
		go func(c resource) {
			defer wg.Done()
			m.handleChapter(c)
		}(c)
	}
	wg.Wait()
}

func (m *MangaEdenCrawler) handleChapter(chapter resource) {
	chapterDoc, err := m.client.GetHTML(chapter.url)
	if err != nil {
		log.Fatal(err)
	}

	if chapter.info == nil {
		pathParts := strings.Split(chapter.url.EscapedPath(), "/")
		pathParts = pathParts[3:] // discard the /en/en-manga/

		chapterPath := chapter.url.EscapedPath()
		mangaURL, _ := chapter.url.Parse(".")
		if len(pathParts) == 3 {
			// chapter url with ending slash (one-piece/2/)
			// or page url (one-piece/2/3)
			chapterPath = path.Dir(chapterPath)
			mangaURL, _ = chapter.url.Parse("..")
		} else if len(pathParts) == 4 {
			// page url with ending slash (one-piece/2/3/)
			chapterPath = path.Dir(path.Dir(chapterPath))
			mangaURL, _ = chapter.url.Parse("../..")
		}

		mangaDoc, err := m.client.GetHTML(mangaURL)
		if err != nil {
			log.Fatal(err)
		}
		allChapters := m.scraper.GetChapters(mangaDoc)

		for _, c := range allChapters {
			// in manga pages, urls are always page urls with ending slashes
			p := path.Dir(path.Dir(c.url.EscapedPath()))
			if chapterPath == p {
				chapter.info = c.info
				break
			}
		}
	}

	fmt.Println(chapter.info)

	otherPages, thisPage := m.scraper.GetPages(chapterDoc)
	thisPage[0].info.Update(chapter.info)
	for i := 0; i < len(otherPages); i++ {
		otherPages[i].info.Update(chapter.info)
	}

	if m.rule.Block(thisPage[0].info) {
		return
	}

	m.obs.OnChapterStart(thisPage[0].info)
	m.obs.OnPageStart(thisPage[0].info)
	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		m.handleImage(thisPage[0])
	}()

	for _, p := range otherPages {
		wg.Add(1)
		go func(p resource) {
			defer wg.Done()
			m.handlePage(p)
		}(p)
	}

	wg.Wait()
	m.obs.OnPageEnd(thisPage[0].info)
	m.obs.OnChapterEnd(thisPage[0].info)
}

func (m *MangaEdenCrawler) handlePage(page resource) resource {
	m.obs.OnPageStart(page.info)

	pageDoc, err := m.client.GetHTML(page.url)
	if err != nil {
		log.Fatal(err)
	}
	img := m.scraper.GetImage(pageDoc)
	img.info.Update(page.info)
	defer m.obs.OnPageEnd(img.info)

	if err := m.handleImage(img); err != nil {
		log.Fatal(err)
	}
	return img
}

func (m *MangaEdenCrawler) handleImage(img resource) error {
	r, err := m.client.Get(img.url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	out, err := m.saver.Save(img.info, r.ContentLength)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, r.Body); err != nil {
		return err
	}
	return nil
}

type MangaEdenHandler struct{}

func (m MangaEdenHandler) CanHandle(u *url.URL) bool {
	// netlocs := []string{
	// 	"mangareader.net",
	// 	"www.mangareader.net",
	// }

	// for _, h := range netlocs {
	// 	if strings.Contains(u.Hostname(), h) {
	// 		return true
	// 	}
	// }
	// return false

	return strings.Contains(u.Hostname(), "mangaeden.com")
}

func (m MangaEdenHandler) Handle(u *url.URL, fetcher Fetcher, saver Saver, rule Rule, obs Observer) {
	if !m.CanHandle(u) {
		log.Fatalln("mangaeden: do not recognize", u)
	}

	crawler := MangaEdenCrawler{
		client: fetcher,
		saver:  saver,
		rule:   rule,
		obs:    obs,
	}

	// XXX: clean up the url; will also make handleChapter easier
	cleanPath := path.Clean(u.EscapedPath())
	pathParts := strings.Split(cleanPath, "/")
	pathParts = pathParts[3:] // discard the /en/en-manga/ part
	if len(pathParts) == 1 {
		crawler.handleManga(u)
	} else if len(pathParts) == 2 || len(pathParts) == 3 {
		crawler.handleChapter(resource{u, nil})
	} else {
		log.Fatalln("mangaeden: cannot handle", u)
	}
}
