package main

import (
	"log"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type MangaStreamerScraper struct{}

func (m MangaStreamerScraper) GetChapters(doc *goquery.Document) (chapters []Resource) {
	mangainfo := Metadata{
		"manga":            doc.Find("h1").Text(),
		"readingDirection": "rtl",
	}

	mangaName := mangainfo["manga"].(string)
	if len(mangaName) < 1 {
		log.Fatal("cannot extract chapters: no manga name")
	}

	links := doc.Find("table a")
	mangainfo["chapters"] = links.Length()

	links.Each(func(i int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			log.Fatal("cannot extract chapters: no link")
		}

		re := regexp.MustCompile(`(?P<num>[^-]*)(?: - (?P<name>.*))?`)
		match := re.FindStringSubmatch(s.Text())
		if len(match) < 1 {
			log.Fatal("cannot extract chapters: no number")
		}

		chapterinfo := Metadata{
			"chapterIndex": i + 1,
			"chapter":      match[1],
			"chapterName":  match[2],
			// "dateAdded":    s.Next().Text(),
		}
		chapterinfo.Update(mangainfo)

		allDigits := false
		for _, c := range chapterinfo["chapter"].(string) {
			allDigits = '0' <= c && c <= '9'
		}
		if allDigits {
			chapterinfo["chapter"], _ = strconv.Atoi(chapterinfo["chapter"].(string))
		}

		u, err := doc.Url.Parse(href)
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

func (m MangaStreamerScraper) isSamePage(a, fromUser *url.URL) bool {
	aPath := a.EscapedPath()
	userPath := strings.TrimRight(fromUser.EscapedPath(), "/")

	if ok, err := path.Match("/r*/*/*/*/[0-9]*", aPath); !ok || err != nil {
		log.Fatalln("invalid page url")
	}

	switch strings.Count(userPath, "/") {
	default:
		n := 0
		for i := 0; i < len(userPath); i++ {
			if userPath[i] == '/' {
				n++
			}
			if n >= 6 {
				userPath = userPath[:i]
			}
		}
		fallthrough
	case 5:
		return path.Base(userPath) == path.Base(aPath)
	case 4:
		return path.Base(aPath) == "1"
	}
}

func (m MangaStreamerScraper) GetPages(doc *goquery.Document) (pages []Resource, images []Resource) {
	links := doc.Find(".btn-primary + .dropdown-menu a")
	links.Each(func(i int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			log.Fatal("cannot extract pages: no link")
		}

		info := Metadata{
			"pages":     links.Length(),
			"pageIndex": i + 1,
		}

		u, err := doc.Url.Parse(href)
		if err != nil {
			log.Fatalln("cannot extract pages:", err)
		}
		if m.isSamePage(u, doc.Url) {
			img := m.GetImage(doc)
			img.info.Update(info)
			images = append(images, img)
		} else {
			pages = append(pages, Resource{u, info})
		}
	})
	return
}

func (m MangaStreamerScraper) GetImage(doc *goquery.Document) Resource {
	imgSrc, ok := doc.Find("#manga-page").Attr("src")
	if !ok {
		log.Fatal("cannot extract image: no #img or @src")
	}

	imgURL, err := doc.Url.Parse(imgSrc)
	if err != nil {
		log.Fatalln("cannot extract image:", err)
	}
	return Resource{imgURL, Metadata{
		"imageExtension": path.Ext(imgURL.EscapedPath())[1:],
	}}
}

type MangaStreamerCrawler struct {
	CommonSimpleCrawler
}

func NewMangaStreamerCrawler(fetcher Fetcher, saver Saver, rule Rule, obs Observer) *MangaStreamerCrawler {
	crawler := &MangaStreamerCrawler{
		CommonSimpleCrawler{
			scraper: MangaStreamerScraper{},
			client:  fetcher,
			saver:   saver,
			rule:    rule,
			obs:     obs,
		},
	}

	return crawler
}

func (m *MangaStreamerCrawler) Handle(u *url.URL) {
	cleanPath := strings.TrimRight(u.EscapedPath(), "/")

	mangaURL := u
	switch strings.Count(cleanPath, "/") {
	case 5:
		// page url (/read/one_piece/917/5340/3)
		cleanPath = path.Dir(cleanPath)
		fallthrough
	case 4:
		// chapter url (/read/one_piece/917/5340)
		chapterPath := cleanPath

		// There's actually no reliable way to extract a URL to the manga from
		// a chapter URL; mangastream assigns a unique ID to each chapter and
		// uses that to decice which chapter to show.  Nevertheless, we'll try
		// and and uses the name of the manga from the URL and if it doesn't
		// actually exist, we'll fail.
		mangaName := path.Base(path.Dir(path.Dir(chapterPath)))
		mangaURL, _ = u.Parse("/manga/" + mangaName)

		// add a rule to only download the requested chapter
		whitelistRule := funcRule(func(r Resource) bool {
			cleanPath := strings.TrimRight(r.url.EscapedPath(), "/")
			if cleanPath[:2] == "/r" {
				chapterID := path.Base(path.Dir(cleanPath))
				return path.Base(chapterPath) != chapterID
			}
			return false
		})
		m.rule = AndRule{whitelistRule, m.rule}
		fallthrough
	case 2:
		// manga url (/manga/one_piece)
		m.handleManga(mangaURL)

	default:
		log.Fatalln("mangastream: cannot handle", u)
	}
}
