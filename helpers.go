package main

import (
	"io"
	"log"
	"os"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type empty struct{}

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

type ProgressReader struct {
	Reader   io.Reader
	Size     int64
	Callback func(int64, int64)

	progress int64
}

func (p *ProgressReader) Read(buf []byte) (int, error) {
	read, err := p.Reader.Read(buf)

	if p.Size != 0 {
		oldProgress := p.progress
		oldPercent := (100 * oldProgress) / p.Size
		p.progress += int64(read)
		percent := (100 * p.progress) / p.Size
		if percent > oldPercent && p.Callback != nil {
			p.Callback(p.progress, p.Size)
		}
	} else {
		p.Callback(p.progress, p.Size)
	}

	return read, err
}

type ProgressWriter struct {
	Writer   io.WriteCloser
	Size     int64
	Callback func(int64, int64)

	progress int64
}

func (p *ProgressWriter) Write(buf []byte) (int, error) {
	count, err := p.Writer.Write(buf)

	p.progress += int64(count)
	if p.Callback != nil {
		p.Callback(p.progress, p.Size)
	}
	return count, err
}

func (p *ProgressWriter) Close() error {
	return p.Writer.Close()
}
