// An ad-hoc parser for Wikipedia's 45GB (and growing) XML database.
package parse

import (
	"bufio"
	"encoding/xml"
	"io"
	"log"
	"regexp"
	"strings"
)

var pageStartRegex = regexp.MustCompile(".*<page>.*")
var pageEndRegex = regexp.MustCompile(".*</page>.*")
var redirectRegex = regexp.MustCompile("#REDIRECT[ \t].*?\\[\\[.*?\\]\\]")
var linkRegex = regexp.MustCompile("\\[\\[([^|]+?)\\]\\]")
var categoryRegex = regexp.MustCompile("\\[\\[Category:(.+?)\\]\\]")

// Parse parses given reader as XML and dumps Page objects with links
// into its output channel.
func Parse(reader io.Reader, pages chan<- *Page) {
	chunks := make(chan []byte)
	rawPages := make(chan []byte)
	nonRedirectPages := make(chan []byte)
	somePages := make(chan *Page)

	go GetChunks(reader, chunks)
	go GetRawPages(chunks, rawPages)
	go FilterRedirects(rawPages, nonRedirectPages)
	go GetPages(nonRedirectPages, somePages)
	go GetLinks(somePages, pages)
}

// CategorizedParse is just like Parse, except that it also categorizes pages.
func CategorizedParse(reader io.Reader, out chan<- *Page) {
	pages := make(chan *Page)

	go GetCategories(pages, out)

	Parse(reader, pages)
}

// GetChunks reads an XML file line by line and dumps each line to its output channel.
func GetChunks(reader io.Reader, chunks chan<- []byte) {
	scanner := bufio.NewScanner(reader)

	eof := false

	for !eof {
		if scanner.Scan() {
			chunks <- scanner.Bytes()
		} else {
			if err := scanner.Err(); err != nil {
				log.Println(err.Error() + " skipping line")
			} else {
				eof = true
			}
		}
	}

	close(chunks)
}

// GetRawPages combines individual line elements into complete XML pages
// so that they can be processed by a standard in-memory XML parser.
func GetRawPages(chunks <-chan []byte, pages chan<- []byte) {
	page := []byte("<page>")
	inPage := false

	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				close(pages)
				return
			}

			if pageStartRegex.Match(chunk) {
				inPage = true
			} else if pageEndRegex.Match(chunk) {
				if inPage {
					page = append(page, []byte("</page>")...)
					pages <- page
					page = []byte("<page>")
				}

				inPage = false
			} else {
				if inPage {
					page = append(page, chunk...)
				}
			}
		}
	}
}

// FilterRedirects discards all pages that redirect to another page.
func FilterRedirects(rawPages <-chan []byte, nonRedirectPages chan<- []byte) {
	for {
		select {
		case rawPage, ok := <-rawPages:
			if !ok {
				close(nonRedirectPages)
				return
			}

			if redirectRegex.Find(rawPage) == nil {
				nonRedirectPages <- rawPage
			}
		}
	}
}

// GetPages parses a complete XML page into a page object.
func GetPages(rawPages <-chan []byte, pages chan<- *Page) {
	for {
		select {
		case rawPage, ok := <-rawPages:
			if !ok {
				close(pages)
				return
			}

			pageStruct := &Page{}

			err := xml.Unmarshal(rawPage, pageStruct)

			if err != nil {

			} else {
				pages <- pageStruct
			}
		}
	}
}

// GetLinks extracts all Wikipedia links found in pages.
// Only links in the form [[target]] are extracted.
func GetLinks(pages <-chan *Page, linkedPages chan<- *Page) {
	for {
		select {
		case page, ok := <-pages:
			if !ok {
				close(linkedPages)
				return
			}

			links := linkRegex.FindAllStringSubmatch(page.Revision.Text, -1)

			for _, link := range links {
				page.Links = append(page.Links, link[1])
			}

			linkedPages <- page
		}
	}
}

// GetCategories extracts categories out of each Wikipedia page
// and adds them to the given categories object.
// Only links in the form [[Category:target]] are extracted.
func GetCategories(pages <-chan *Page, categorizedPages chan<- *Page) {
	for {
		select {
		case page, ok := <-pages:
			if !ok {
				close(categorizedPages)
				return
			}

			rawCats := categoryRegex.FindAllStringSubmatch(page.Revision.Text, -1)
			cats := make([]string, len(rawCats))

			for i, rawCat := range rawCats {
				cats[i] = strings.Trim(rawCat[1], " \t|")
			}

			page.Categories = cats

			categorizedPages <- page
		}
	}
}
