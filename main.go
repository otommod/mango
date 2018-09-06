package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
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
	transport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,

		TLSHandshakeTimeout: 10 * time.Second,
		// ResponseHeaderTimeout: 10 * time.Second,
		// ExpectContinueTimeout: 1 * time.Second,

		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 200,
		IdleConnTimeout:     90 * time.Second,
	}
	client = &http.Client{
		Transport: transport,
		// Timeout:   15 * time.Second,
	}
)

type Metadata map[string]interface{}

func (m Metadata) Update(other Metadata) {
	for k, v := range other {
		m[k] = v
	}
}

type Handler interface {
	Subscribe(Observer)
	Handle(context.Context, *url.URL)
}

type Saver interface {
	Save(info Metadata, size int64) (io.WriteCloser, error)
}

type Rule interface {
	Block(Metadata) bool
}

type Observer interface {
	OnPageEnd(Metadata)
	OnChapterEnd(Metadata)
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

func (f Fetcher) Get(ctx context.Context, u *url.URL) (*http.Response, error) {
	for _, r := range f.domainRules {
		if r.domain.Match(u.Hostname()) {
			r.semaphore <- empty{}
			defer func() { <-r.semaphore }()
			<-r.rateLimiter
			break
		}
	}

	log.Println("GET", u)

	req, _ := http.NewRequest("GET", u.String(), nil)
	// trace := &httptrace.ClientTrace{
	// 	GotConn: func(connInfo httptrace.GotConnInfo) {
	// 		fmt.Printf("Got Conn: %+v\n", connInfo)
	// 	},
	// 	DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
	// 		fmt.Printf("DNS Info: %+v\n", dnsInfo)
	// 	},
	// }
	// req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	// req = req.WithContext(WithHTTPStat(ctx, &RequestTimings{}))
	req = req.WithContext(ctx)

	r, err := f.client.Do(req)
	if err == nil && r.StatusCode != 200 {
		// XXX: find a nicer way to do error codes
		return nil, fmt.Errorf("GET %s: %d", u.String(), r.StatusCode)
	}
	return r, err
}

func (f Fetcher) GetHTML(ctx context.Context, u *url.URL) (*goquery.Document, error) {
	page, err := f.Get(ctx, u)
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
	chaptersLen := len(strconv.Itoa(info["chapters"].(int)))
	pagesLen := len(strconv.Itoa(info["pages"].(int)))

	dirname = fmt.Sprintf("%s/%0*d",
		info["manga"],
		chaptersLen,
		info["chapter"])
	basename = fmt.Sprintf("%0*d.%s",
		pagesLen,
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

	task := s.progressBar.StartTask(size)
	return &ProgressWriter{
		Writer: file,
		Size:   size,
		Callback: func(prog, total int64) {
			s.progressBar.TickTask(task, prog)
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

func (s PageSaver) Block(info Metadata) bool {
	dirname, _ := s.name(info)
	return isDir(dirname)
}

type CBZSaver struct {
	progressBar *ProgressBar
}

func (s CBZSaver) name(info Metadata) (archivename, imagename string) {
	chaptersLen := len(strconv.Itoa(info["chapters"].(int)))
	pagesLen := len(strconv.Itoa(info["pages"].(int)))

	archivename = fmt.Sprintf("%s/%0*d.cbz",
		info["manga"],
		chaptersLen,
		info["chapter"])
	imagename = fmt.Sprintf("%0*d.%s",
		pagesLen,
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

	task := s.progressBar.StartTask(size)
	return &ProgressWriter{
		Writer: file,
		Size:   size,
		Callback: func(prog, total int64) {
			s.progressBar.TickTask(task, prog)
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

func (s CBZSaver) Block(m Metadata) bool {
	archivename, _ := s.name(m)
	return isFile(archivename)
}

func handler(u *url.URL, fetcher Fetcher, saver Saver, rule Rule) Handler {
	switch {
	case strings.Contains(u.Hostname(), "mangareader.net"):
		return NewMangaReaderCrawler(fetcher, saver, rule)
		// case strings.Contains(u.Hostname(), "mangaeden.com"):
		// 	return NewMangaEdenCrawler(fetcher, saver, rule)
	}
	return nil
}

func main() {
	progressBar := NewProgressBar()
	defer progressBar.Stop()

	ctx := context.Background()

	// trap Ctrl+C and call cancel on the context
	ctx, cancel := context.WithCancel(ctx)

	sigs := make(chan os.Signal)
	signal.Notify(sigs, os.Interrupt)
	defer func() {
		signal.Stop(sigs)
		cancel()
	}()
	go func() {
		select {
		case <-sigs:
			cancel()
		case <-ctx.Done():
		}
	}()

	fetcher := NewFetcher(50, 10)
	saver := CBZSaver{progressBar: progressBar}
	// rule := saver
	rule := AndRule{saver, LastChapterRule{}}

	wg := sync.WaitGroup{}

	chapters := os.Args[1:]
	for _, c := range chapters {
		u, err := url.Parse(c)
		if err != nil {
			log.Fatal(err)
		}

		h := handler(u, fetcher, saver, rule)
		wg.Add(1)
		h.Subscribe(saver)
		go func() {
			defer wg.Done()
			h.Handle(ctx, u)
		}()
	}

	wg.Wait()
}
