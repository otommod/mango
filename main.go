package main

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gobwas/glob"
)

var (
	// Customize the Transport to have larger connection pool
	// transport = &http.Transport{
	// 	MaxIdleConns:        100,
	// 	MaxIdleConnsPerHost: 8,
	// }
	client = &http.Client{
		Transport: http.DefaultTransport,
	}
)

type Metadata map[string]interface{}

func (m Metadata) Update(other Metadata) {
	for k, v := range other {
		m[k] = v
	}
}

type Resource struct {
	url  *url.URL
	info Metadata
}

type Handler interface {
	Handle(*url.URL)
}

type Saver interface {
	Save(info Metadata, size int64) (io.WriteCloser, error)
}

type Rule interface {
	Block(Resource) bool
}

type Observer interface {
	OnChapterEnd(Metadata)
	OnPageEnd(Metadata)
}

type domainRule struct {
	domain      glob.Glob
	semaphore   chan empty
	rateLimiter <-chan time.Time
}

type Fetcher struct {
	client      *http.Client
	domainRules []domainRule
}

func NewFetcher(maxConnections, perSecond int) Fetcher {
	f := Fetcher{client: client}
	f.Limit("*", maxConnections, perSecond)
	return f
}

func (f *Fetcher) Limit(domainGlob string, maxConnections, perSecond int) {
	f.domainRules = append(f.domainRules, domainRule{
		glob.MustCompile(domainGlob),
		make(chan empty, maxConnections),
		time.Tick(time.Second / time.Duration(perSecond)),
	})
}

func (f Fetcher) Get(u *url.URL) (*http.Response, error) {
	for _, r := range f.domainRules {
		if r.domain.Match(u.Hostname()) {
			r.semaphore <- empty{}
			defer func() { <-r.semaphore }()
			<-r.rateLimiter
			break
		}
	}

	log.Println("GET", u)
	r, err := f.client.Get(u.String())
	if err == nil && r.StatusCode != 200 {
		// XXX: find a nicer way to do error codes
		return nil, fmt.Errorf("GET %s: %d", u.String(), r.StatusCode)
	}
	return r, err
}

func (f Fetcher) GetHTML(u *url.URL) (*goquery.Document, error) {
	page, err := f.Get(u)
	if err != nil {
		return nil, err
	}

	// XXX: don't use NewDocumentFromResponse
	return goquery.NewDocumentFromResponse(page)
}

type PageSaver struct {
	progressBar *ProgressBar
}

func (s PageSaver) name(info Metadata) (dirname, basename string) {
	if chapters, ok := info["chapters"].(int); ok {
		dirname = fmt.Sprintf("%s/%0*d", info["manga"],
			len(strconv.Itoa(chapters)), info["chapter"])
	}
	if pages, ok := info["pages"].(int); ok {
		basename = fmt.Sprintf("%0*d.%s",
			len(strconv.Itoa(pages)), info["page"], info["imageExtension"])
	}
	return
}

func (s PageSaver) Save(info Metadata, size int64) (io.WriteCloser, error) {
	dirname, basename := s.name(info)
	tmpdirname, tmpbasename := dirname+".part", basename+".part"

	os.MkdirAll(tmpdirname, os.ModeDir|0770)

	tmpname := filepath.Join(tmpdirname, tmpbasename)
	file, err := os.Create(tmpname)
	if err != nil {
		return nil, err
	}

	task := s.progressBar.NewTask()
	return &ProgressWriter{
		Writer: file,
		Size:   size,
		Callback: func(sofar, total int64) {
			s.progressBar.TickTask(task, sofar, total)
		},
	}, nil
}

func (s PageSaver) OnPageEnd(info Metadata) {
	dirname, basename := s.name(info)
	tmpdirname, tmpbasename := dirname+".part", basename+".part"

	tmpname := filepath.Join(tmpdirname, tmpbasename)
	if isFile(tmpname) {
		os.Rename(tmpname, filepath.Join(tmpdirname, basename))
	} else {
		// shouldn't happen
	}
}

func (s PageSaver) OnChapterEnd(info Metadata) {
	dirname, _ := s.name(info)
	tmpdirname := dirname + ".part"

	if isDir(tmpdirname) {
		os.Rename(tmpdirname, dirname)
	} else {
		// shouldn't happen
	}
}

func (s PageSaver) Block(r Resource) bool {
	dirname, _ := s.name(r.info)
	return isDir(dirname)
}

type CBZSaver struct {
	progressBar *ProgressBar
}

func (s CBZSaver) name(info Metadata) (archivename, imagename string) {
	if chapters, ok := info["chapters"].(int); ok {
		archivename = fmt.Sprintf("%s/%0*d.cbz",
			info["manga"], len(strconv.Itoa(chapters)), info["chapter"])
	}
	if pages, ok := info["pages"].(int); ok {
		imagename = fmt.Sprintf("%0*d.%s",
			len(strconv.Itoa(pages)), info["page"], info["imageExtension"])
	}
	return
}

func (s CBZSaver) Save(info Metadata, size int64) (io.WriteCloser, error) {
	archivename, imagename := s.name(info)
	tmparchivename, tmpimagename := archivename+".part", imagename+".part"

	os.MkdirAll(tmparchivename, os.ModeDir|0770)

	tmpname := filepath.Join(tmparchivename, tmpimagename)
	file, err := os.Create(tmpname)
	if err != nil {
		return nil, err
	}

	task := s.progressBar.NewTask()
	return &ProgressWriter{
		Writer: file,
		Size:   size,
		Callback: func(sofar, total int64) {
			s.progressBar.TickTask(task, sofar, total)
		},
	}, nil
}

func (s CBZSaver) OnPageEnd(info Metadata) {
	archivename, imagename := s.name(info)
	tmparchivename, tmpimagename := archivename+".part", imagename+".part"

	tmpname := filepath.Join(tmparchivename, tmpimagename)
	if isFile(tmpname) {
		os.Rename(tmpname, filepath.Join(tmparchivename, imagename))
	} else {
		// shouldn't happen
	}
}

func (s CBZSaver) OnChapterEnd(info Metadata) {
	archivename, _ := s.name(info)
	tmparchivename := archivename + ".part"

	zipfile, err := os.Create(archivename)
	if err != nil {
		log.Fatal(err)
	}
	defer zipfile.Close()

	archive := zip.NewWriter(zipfile)
	defer archive.Close()

	filepath.Walk(tmparchivename, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		} else if info.IsDir() {
			// this shouldn't happen but whatever
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Name = strings.TrimPrefix(path, tmparchivename+"/")
		header.Method = zip.Deflate

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})
}

func (s CBZSaver) Block(r Resource) bool {
	archivename, _ := s.name(r.info)
	return isFile(archivename)
}

func handler(u *url.URL, fetcher Fetcher, saver Saver, rule Rule, obs Observer) Handler {
	switch {
	case strings.Contains(u.Hostname(), "mangareader.net"):
		return NewMangaReaderCrawler(fetcher, saver, rule, obs)
	case strings.Contains(u.Hostname(), "mangaeden.com"):
		return NewMangaEdenCrawler(fetcher, saver, rule, obs)
	}
	return nil
}

func main() {
	progressBar := NewProgressBar()
	defer progressBar.Stop()

	fetcher := NewFetcher(50, 10)
	saver := CBZSaver{progressBar: progressBar}
	rule := saver
	// rule := AndRule{saver, LastChapterRule{}}

	wg := sync.WaitGroup{}

	chapters := os.Args[1:]
	for _, c := range chapters {
		u, err := url.Parse(c)
		if err != nil {
			log.Fatal(err)
		}

		h := handler(u, fetcher, saver, rule, saver)
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.Handle(u)
		}()
	}

	wg.Wait()
}
