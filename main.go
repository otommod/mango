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
	"runtime"
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

	handlers = []Handler{
		MangaReaderHandler{},
	}
)

type Metadata map[string]interface{}

func (m Metadata) Update(other Metadata) {
	for k, v := range other {
		m[k] = v
	}
}

type Handler interface {
	CanHandle(*url.URL) bool
	Handle(*url.URL, Fetcher, Saver, Rule, Observer)
}

type Saver interface {
	Save(info Metadata, size int64) (io.WriteCloser, error)
}

type Rule interface {
	Block(Metadata) bool
}

type Observer interface {
	OnChapterStart(Metadata)
	OnChapterEnd(Metadata)
	OnPageStart(Metadata)
	OnPageEnd(Metadata)
}

type empty struct{}

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
		return nil, fmt.Errorf("GET %s: %s", u.String(), r.Status)
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

func isDir(path string) bool {
	finfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		log.Fatal(err)
	}
	return finfo.IsDir()
}

func isFile(path string) bool {
	finfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		log.Fatal(err)
	}
	// There are more things than directories that are not files (e.g. sockets,
	// devices, etc)
	return !finfo.IsDir()
}

type PageSaver struct{}

func (s PageSaver) name(info Metadata) (dirname, basename string) {
	dirname = fmt.Sprintf("%s/%0*d",
		info["manga"],
		info["chapters_len"],
		info["chapter"])
	basename = fmt.Sprintf("%0*d.%s",
		info["pages_len"],
		info["page"],
		info["image_ext"])
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

	task := progressBar.StartTask(size)
	return &ProgressWriter{
		Writer: file,
		Size:   size,
		Callback: func(prog, total int64) {
			progressBar.TickTask(task, prog)
		},
	}, nil
}

func (s PageSaver) OnPageStart(_ Metadata)    {}
func (s PageSaver) OnChapterStart(_ Metadata) {}

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

func (s PageSaver) Block(info Metadata) bool {
	dirname, _ := s.name(info)

	if isDir(dirname) {
		log.Println("blocking", dirname)
		return true
	}
	log.Println("allowing", dirname)
	return false
}

type CBZSaver struct{}

func (s CBZSaver) name(info Metadata) (archivename, imagename string) {
	archivename = fmt.Sprintf("%s/%0*d.cbz",
		info["manga"],
		info["chapters_len"],
		info["chapter"])
	imagename = fmt.Sprintf("%0*d.%s",
		info["pages_len"],
		info["page"],
		info["image_ext"])
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

	task := progressBar.StartTask(size)
	return &ProgressWriter{
		Writer: file,
		Size:   size,
		Callback: func(prog, total int64) {
			progressBar.TickTask(task, prog)
		},
	}, nil
}

func (s CBZSaver) OnPageStart(_ Metadata)    {}
func (s CBZSaver) OnChapterStart(_ Metadata) {}

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

func (s CBZSaver) Block(m Metadata) bool {
	archivename, _ := s.name(m)

	if isFile(archivename) {
		log.Println("blocking", archivename)
		return true
	}
	log.Println("allowing", archivename)
	return false
}

// type CombinedObserver struct {
// 	observers []Observer
// }

// func (co *CombinedObserver) OnChapterStart(id int) {
// 	for _, o := range co.observers {
// 		o.OnChapterStart(id)
// 	}
// }

// func (co *CombinedObserver) OnChapterEnd(id int) {
// 	for _, o := range co.observers {
// 		o.OnChapterEnd(id)
// 	}
// }

// func (co *CombinedObserver) OnPageStart(id int) {
// 	for _, o := range co.observers {
// 		o.OnPageStart(id)
// 	}
// }

// func (co *CombinedObserver) OnPageEnd(id int) {
// 	for _, o := range co.observers {
// 		o.OnPageEnd(id)
// 	}
// }

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	fetcher := NewFetcher(50, 10)
	saver := CBZSaver{}

	wg := sync.WaitGroup{}

	chapters := os.Args[1:]
	for _, c := range chapters {
		u, err := url.Parse(c)
		if err != nil {
			log.Fatal(err)
		}

		for _, h := range handlers {
			if h.CanHandle(u) {
				wg.Add(1)
				go func() {
					defer wg.Done()
					h.Handle(u, fetcher, saver, saver, saver)
				}()
				break
			}
		}
	}

	wg.Wait()
	progressBar.Stop()
}
