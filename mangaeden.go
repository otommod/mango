package main

import (
	"log"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type MangaEdenScraper struct{}

func (m MangaEdenScraper) GetChapters(doc *goquery.Document) (chapters []Resource) {
	comicType := nextTextNode(doc.Find("#rightContent h4:contains('Type')")).Text()
	comicType = strings.ToLower(strings.TrimSpace(comicType))
	readingDirection := "ltr"
	if comicType == "japanese manga" || comicType == "chinese manhua" || comicType == "doujinshi" {
		readingDirection = "rtl"
	}

	status := nextTextNode(doc.Find("#rightContent h4:contains('Status')")).Text()
	status = strings.TrimSpace(status)

	mangainfo := Metadata{
		"manga":            doc.Find(".manga-title").Text(),
		"author":           doc.Find("#rightContent h4:contains('Author') + a").Text(),
		"artist":           doc.Find("#rightContent h4:contains('Artist') + a").Text(),
		"status":           status,
		"readingDirection": readingDirection,
		"genres":           doc.Find("#rightContent h4:contains('Genres') ~ a").Map(mapSelectionText),
		"description":      doc.Find("#mangaDescription").Text(),
		"coverImage":       doc.Find(".mangaImage2 img").AttrOr("src", ""),
	}

	mangaName := mangainfo["manga"].(string)
	if len(mangaName) < 1 {
		log.Fatal("cannot extract chapters: no manga name")
	}

	chapterLinks := doc.Find(".chapterLink")
	mangainfo["chapters"] = chapterLinks.Length()

	chapterLinks.Each(func(i int, s *goquery.Selection) {
		if goquery.NodeName(s) != "a" {
			log.Fatal("cannot extract chapters: no link")
		}
		link, ok := s.Attr("href")
		if !ok {
			log.Fatal("cannot extract chapters: no link")
		}

		re := regexp.MustCompile(`(?P<num>[^:]+)(?:: (?P<name>.*))?`)
		// match := re.FindStringSubmatch(strings.TrimLeftFunc(s.Text(), unicode.IsSpace))
		match := re.FindStringSubmatch(s.Find("b").Text())
		if len(match) < 1 {
			log.Fatal("cannot extract chapters: no number")
		}

		chapterinfo := Metadata{
			"chapterIndex": i + 1,
			"chapter":      match[1],
			"chapterName":  match[2],
			// "dateAdded":    s.Parent().Parent().Find("td.chapterDate").Text(),
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

func (m MangaEdenScraper) GetPages(doc *goquery.Document) (pages []Resource, images []Resource) {
	options := doc.Find("#pageSelect option")
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

func (m MangaEdenScraper) GetImage(page *goquery.Document) (img Resource) {
	imgSrc, ok := page.Find("#mainImg").Attr("src")
	if !ok {
		log.Fatal("cannot extract image: no #img or @src")
	}

	imgURL, err := page.Url.Parse(imgSrc)
	if err != nil {
		log.Fatalln("cannot extract image:", err)
	}
	return Resource{imgURL, Metadata{"imageExtension": "jpg"}} // XXX: are all images jpgs
}

type MangaEdenCrawler struct {
	CommonSimpleCrawler
}

func NewMangaEdenCrawler(fetcher Fetcher, saver Saver, rule Rule, obs Observer) *MangaEdenCrawler {
	crawler := &MangaEdenCrawler{
		CommonSimpleCrawler{
			scraper: MangaEdenScraper{},
			client:  fetcher,
			saver:   saver,
			rule:    rule,
			obs:     obs,
		},
	}

	return crawler
}

func (m *MangaEdenCrawler) Handle(u *url.URL) {
	cleanPath := strings.TrimRight(u.EscapedPath(), "/")

	mangaURL := u
	switch strings.Count(cleanPath, "/") {
	case 5:
		// page url (/en/en-manga/one-piece/1/1)
		cleanPath = path.Dir(cleanPath)
		fallthrough
	case 4:
		// chapter url (/en/en-manga/one-piece/1)
		chapterPath := cleanPath
		mangaURL, _ = u.Parse(path.Dir(chapterPath))

		// add a rule to only download the requested chapter
		whitelistRule := funcRule(func(r Resource) bool {
			cleanPath := strings.TrimRight(r.url.EscapedPath(), "/")
			return cleanPath != chapterPath && !strings.HasPrefix(cleanPath, chapterPath+"/")
		})
		m.rule = AndRule{whitelistRule, m.rule}
		fallthrough
	case 3:
		// manga url (/en/en-manga/one-piece)
		m.handleManga(mangaURL)

	default:
		log.Fatalln("mangaeden: cannot handle", u)
	}
}
