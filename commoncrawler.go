package main

import (
	"io"
	"log"
	"net/url"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

type Scraper interface {
	GetChapters(*goquery.Document) (chapters []Resource)
	GetPages(*goquery.Document) (pages []Resource, images []Resource)
	GetImage(*goquery.Document) (img Resource)
}

type CommonSimpleCrawler struct {
	scraper Scraper
	client  Fetcher
	saver   Saver
	rule    Rule
	obs     Observer
}

func (m *CommonSimpleCrawler) handleManga(mangaURL *url.URL) {
	mangaDoc, err := m.client.GetHTML(mangaURL)
	if err != nil {
		log.Fatal(err)
	}

	wg := sync.WaitGroup{}
	chapters := m.scraper.GetChapters(mangaDoc)
	for _, c := range chapters {
		wg.Add(1)
		go func(c Resource) {
			defer wg.Done()
			m.handleChapter(c)
		}(c)
	}
	wg.Wait()
}

func (m *CommonSimpleCrawler) handleChapter(chapter Resource) {
	if m.rule.Block(chapter) {
		return
	}

	chapterDoc, err := m.client.GetHTML(chapter.url)
	if err != nil {
		log.Fatal(err)
	}

	otherPages, thisPage := m.scraper.GetPages(chapterDoc)
	thisPage[0].info.Update(chapter.info)
	for i := 0; i < len(otherPages); i++ {
		otherPages[i].info.Update(chapter.info)
	}

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()
		m.handleImage(thisPage[0])
	}()

	for _, p := range otherPages {
		wg.Add(1)
		go func(p Resource) {
			defer wg.Done()
			m.handlePage(p)
		}(p)
	}

	wg.Wait()
	m.obs.OnPageEnd(thisPage[0].info)
	m.obs.OnChapterEnd(thisPage[0].info)
}

func (m *CommonSimpleCrawler) handlePage(page Resource) Resource {
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

func (m *CommonSimpleCrawler) handleImage(img Resource) error {
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
