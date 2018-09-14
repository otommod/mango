package main

import (
	"encoding/xml"
)

type coMet Metadata

func (m coMet) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	// http://www.denvog.com/comet/comet-specification/
	var info struct {
		XMLName xml.Name `xml:"http://www.denvog.com/comet/ comet"`

		// <title>
		// Title of the comic. Required element.
		Title string `xml:"title"`

		// <description>
		// Description of the comic’s content.
		Description string `xml:"description,omitempty"`

		// <series>
		// Name of story arc, or collective volume, often consisting of several issues.
		Series string `xml:"series,omitempty"`

		// <issue>
		// Number representing a single comic release. May be one of several
		// issues that comprise a sequence to make up a volume.
		Issue uint `xml:"issue"`

		// <volume>
		// Number representing annual publication or story arc, often
		// consisting of several issues.
		Volume uint `xml:"volume,omitempty"`

		// <publisher>
		// The group, organization, company or individual who is responsible
		// for originating the production of a publication.
		Publisher string `xml:"publisher,omitempty"`

		// <date>
		// Date of print publication. Utilizes the form YYYY-MM-DD defined in a
		// profile of ISO 8601. Required 4-digit year, optional 2-digit month,
		// and optional 2-digit day of month. If the full date is unknown,
		// month and year (YYYY-MM) or just year (YYYY) may be used. In this
		// scheme, for example, the date 1994-11-05 corresponds to November 5,
		// 1994.
		Date string `xml:"date,omitempty"`

		// <genre>
		// The nature or genre of the comic.
		Genres []string `xml:"genre,omitempty"`

		// <character>
		// Use for an entity depicted or portrayed. Multiple common-separated values allowed.
		Characters []string `xml:"character,omitempty"`

		// <isVersionOf>
		// A reference to a related resource. Version relations are those of
		// which the described comic is a version, edition, or adaptation of
		// another resource by the same creator.
		IsVersionOf string `xml:"isVersionOf,omitempty"`

		// <price>
		// The cover price of the comic.
		// Price float32 `xml:"price"`

		// <format>
		// Marvel: Comics come in their single issue format as noted by the
		// word "Comic", but they’re also sometimes available in "Hardcovers"
		// or "Trade Paperbacks" which generally collect several comics at
		// once.
		Format string `xml:"format,omitempty"`

		// <language>
		// Language: The language of the intellectual content of the resource.
		// Accepted values are those in the ISO 639-1 Alpha-2 list (codes
		// composed of 2 letters of the basic Latin alphabet). Examples include
		// “en” for English.
		Language string `xml:"language,omitempty"`

		// <rating>
		// Some comic publishers provide a rating: For example, Marvel
		// provides: A is appropriate for ages nine and up, T+ is appropriate
		// for ages 12 and up and Parental Advisory is appropriate for ages 15
		// and up. Other ratings you may see are self-explanatory.
		Rating string `xml:"rating,omitempty"`

		// <rights>
		// Information on the rights held for the work. Typically the copyright holder.
		Rights string `xml:"rights,omitempty"`

		// <identifier>
		// A way to uniquely identify the resource by means of a string or
		// number conforming to a formal identification system. Most commonly a
		// UPC code or International Standard Book Number (ISBN). The Bar Codes
		// on comic books are actually UPC codes or more fully the comic book
		// UPC code. The UPC code is a way to identify the product which it
		// appears on. The UPC is comprised of the Company Prefix (CP) which is
		// the first 6 to 9 digits, followed by the Item Reference (IR).
		// Additionally comic books will have an additional 5 digits that
		// identify the issue number.
		Identifier string `xml:"identifier,omitempty"`

		// <pages>
		// Number of pages in a given book.
		Pages int `xml:"pages"`

		// <creator>
		// Entity primarily responsible for making the content of the resource.
		Creators []string `xml:"creator,omitempty"`

		// <writer>
		// Author. Person or corporate body chiefly responsible for the
		// intellectual or artistic content of a work.
		Writers []string `xml:"writer,omitempty"`

		// <penciller>
		// Artist. Use for the person who conceives, and perhaps also
		// implements, a design or illustration, usually to accompany a written
		// text.
		Pencillers []string `xml:"penciller,omitempty"`

		// <editor>
		// Use for a person who prepares for publication a work not primarily
		// his/her own, such as by elucidating text, adding introductory or
		// other critical matter, or technically directing an editorial staff.
		Editors []string `xml:"editor,omitempty"`

		// <letterer>
		// Use for a person or organization primarily responsible for choice
		// and arrangement of type used in an item. Variation of “typographer”
		Letterers []string `xml:"letterer,omitempty"`

		// <inker>
		// Use for a person or organization who cuts letters, figures, etc. on
		// a surface, such as a wooden or metal plate, for printing. Varition
		// of “Engraver”
		Inkers []string `xml:"inker,omitempty"`

		// <colorist>
		// Use for a person or organization responsible for the decoration of a
		// work (especially manuscript material) with precious metals or color,
		// usually with elaborate designs and motifs. Variation of
		// “Illuminator”
		Colorists []string `xml:"colorist,omitempty"`

		// <coverDesigner>
		// Person or organization responsible for the graphic design of a book
		// cover, album cover, slipcase, box, container, etc.
		CoverDesigner string `xml:"coverDesigner,omitempty"`

		// <coverImage>
		// A Uniform Resource Identifier (URI) for the image file representing
		// the comic cover. This is usually the image file name. Display
		// devices should supports image in JPEG, GIF, and PNG formats with a
		// RGB color space. The URI must end in “.jpg”, “.gif” or “.png”.
		// Recommend locating image file at the root directory of the archive.
		CoverImage string `xml:"coverImage,omitempty"`

		// <readingDirection>
		// Specifies the base page flow of the comic. Allowed values are “ltr”
		// (left-to-right) or “rtl” (right-to-left). Japanese Manga intended to
		// be read from right to left would typically designate a “rtl” value.
		// If the attribute is not specified, the value is assumed to be ltr
		// (default).
		ReadingDirection string `xml:"readingDirection,omitempty"`
	}

	if manga, ok := m["manga"]; ok {
		info.Title = manga.(string)
	}
	if chapter, ok := m["chapter"]; ok {
		if n, ok := chapter.(int); ok {
			info.Issue = uint(n)
		}
	}
	if author, ok := m["author"]; ok {
		info.Creators = []string{author.(string)}
	}
	if artist, ok := m["artist"]; ok {
		info.Pencillers = []string{artist.(string)}
	}
	if pages, ok := m["pages"]; ok {
		info.Pages = pages.(int)
	}
	if genres, ok := m["genres"]; ok {
		info.Genres = genres.([]string)
	}
	if readingDirection, ok := m["readingDirection"]; ok {
		info.ReadingDirection = readingDirection.(string)
	}

	e.Indent("", "  ")
	return e.Encode(info)
}
