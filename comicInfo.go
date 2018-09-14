package main

import (
	"encoding/xml"
	"strconv"
)

type comicInfo Metadata

func (m comicInfo) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	var info struct {
		XMLName         xml.Name `xml:"ComicInfo"`
		Title           string   `xml:",omitempty"`
		Series          string   `xml:",omitempty"`
		Number          string   `xml:",omitempty"`
		Count           int      `xml:",omitempty"`
		Volume          int      `xml:",omitempty"`
		AlternateSeries string   `xml:",omitempty"`
		AlternateNumber string   `xml:",omitempty"`
		AlternateCount  int      `xml:",omitempty"`
		Summary         string   `xml:",omitempty"`
		Notes           string   `xml:",omitempty"`
		Year            int      `xml:",omitempty"`
		Month           int      `xml:",omitempty"`
		Writer          string   `xml:",omitempty"`
		Penciller       string   `xml:",omitempty"`
		Inker           string   `xml:",omitempty"`
		Colorist        string   `xml:",omitempty"`
		Letterer        string   `xml:",omitempty"`
		CoverArtist     string   `xml:",omitempty"`
		Editor          string   `xml:",omitempty"`
		Publisher       string   `xml:",omitempty"`
		Imprint         string   `xml:",omitempty"`
		Genre           string   `xml:",omitempty"`
		Web             string   `xml:",omitempty"`
		PageCount       int      `xml:",omitempty"`
		LanguageISO     string   `xml:",omitempty"`
		Format          string   `xml:",omitempty"`

		BlackAndWhite string `xml:",omitempty"`
		Manga         string `xml:",omitempty"`

		// Pages       []PageInfo
		// Fonts       []FontInfo
		// ID          GUID
		// Translation GUID
		// Version     GUID

		// TranslationTitle string
		// Translator       string
		// Tags             string
		// Type             ComicType
	}

	// probably always true
	info.Manga = "Yes"
	info.BlackAndWhite = "Yes"

	if manga, ok := m["manga"]; ok {
		info.Title = manga.(string)
	}
	if chapter, ok := m["chapter"]; ok {
		if n, ok := chapter.(int); ok {
			info.Number = strconv.Itoa(n)
		} else if s, ok := chapter.(string); ok {
			info.Number = s
		}
	}
	if author, ok := m["author"]; ok {
		info.Writer = author.(string)
	}
	if artist, ok := m["artist"]; ok {
		info.Penciller = artist.(string)
	}
	if pages, ok := m["pages"]; ok {
		info.PageCount = pages.(int)
	}

	e.Indent("", "  ")
	return e.Encode(info)
}
