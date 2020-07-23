package commands

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

type Element interface {}

type MDFile struct {
	href    string
	name    string
	dir 	MDDir
}

type MDDir struct {
	name string
	href string
}

func (f *MDFile) GetMarkDown() string {
	return fmt.Sprintf("- [ ] [%s](%s)\n", f.name, f.href)
}

func (d *MDDir) GetMarkDown() string {
	return fmt.Sprintf("* ###[%s](%s)\n", d.name, d.href)
}

func IsContains(elements []string, pattern string) bool {
	if len(elements) == 0 {
		return false
	}
	for _, e := range elements {
		match, err := regexp.MatchString(e, pattern)
		if err != nil {
			log.Fatal(err)
		}
		if match {
			return true
		}
	}

	return false
}

func CheckLink(n *html.Node, extension string, ignoreDirs []string) Element {
	var hasRightStyles = false
	var isTrackedFile  = false
	var isDir          = false

	var href    string
	var fname   string
	var dirname string
	var dirhref string

	var pathArray []string

	matchFile := regexp.MustCompile(fmt.Sprintf(".+/blob/.+%s$", extension))
	matchDir := regexp.MustCompile(`.+/tree/.+`)

	for _, a := range n.Attr {
		switch a.Key {
		case "href":
			href = "https://github.com" + a.Val
			fname = n.FirstChild.Data

			pathArray = strings.Split(href, "/")
			if matchDir.Match([]byte(href)) {
				isDir = true
				dirname = strings.Join(pathArray[6:len(pathArray)], "/")
			} else if matchFile.Match([]byte(href)) {
				isTrackedFile = true
				dirname = strings.Join(pathArray[7:len(pathArray)-1], "/")
			}
		case "class":
			if a.Val == "js-navigation-open link-gray-dark" {
				hasRightStyles = true
			}
		}
	}

	if !hasRightStyles {
		return nil
	}	

	if IsContains(ignoreDirs, dirname) {
		return nil
	}

	if isTrackedFile {
		return MDFile {
			href:    href,
			name:    fname[:len(fname)-len(extension)],
			dir: MDDir{
				href: dirhref,
				name: dirname,
			},
		}
	}

	if isDir {
		return MDDir {
			href: href,
			name: dirname,
		}
	}

	return nil
}

func Extract(url, extension string, ignoreDirs []string) []Element {
	res, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}

	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Fatalf("getting %s by HTML: %v", url, res.Status)
	}

	doc, err := html.Parse(res.Body)
	if err != nil {
		log.Fatalf("analise %s by HTML: %v", url, err.Error())
	}

	files := make([]Element, 0)
	visitNode := func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			link := CheckLink(n, extension, ignoreDirs)
			if link != nil {
				files = append(files, link)
			}
		}
	}

	ForEachNode(doc, visitNode)
	return files
}

func ForEachNode(n *html.Node, f func(n *html.Node)) {
	f(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		ForEachNode(c, f)
	}
}

func GroupByDir(files []MDFile) map[MDDir][]MDFile {
	grouped := make(map[MDDir][]MDFile)
	for _, f := range files {
		grouped[f.dir] = append(grouped[f.dir], f)
	}

	return grouped
}

func Crawl(url, extension string, ignoreDirs []string) []MDFile {
	worklist := make(chan []Element)
	results := make([]MDFile, 0)

	// Start with cmd arguments
	go func() {
		worklist <- Extract(url, extension, ignoreDirs)
	}()

	for n := 1; n > 0; n-- {
		list := <-worklist
		for _, f := range list {
			switch v := f.(type) {
			case MDDir:
				n++
				time.Sleep(100 * time.Millisecond)
				go func() {
					worklist <- Extract(v.href, extension, ignoreDirs)
				}()
			case MDFile:
				results = append(results, v)
			}
		}
	}

	return results
}

func PrintResults(out io.Writer, results []MDFile) {
	for dir, files := range GroupByDir(results) {
		_, err := fmt.Fprintf(out, dir.GetMarkDown())
		if err != nil {
			log.Fatal(err)
		}
		for _, f := range files {
			_, err := fmt.Fprint(out, f.GetMarkDown())
			if err != nil {
				log.Fatal(err)
			}
		}
		_, err = fmt.Fprintf(out, "\n")
		if err != nil {
			log.Fatal(err)
		}
	}
}

func getIgnoreDirs(toIgnore string) []string {
	var ignoreDirs = strings.Split(toIgnore, " ")
	if ignoreDirs[0] == "" {
		return make([]string, 0)
	}
	return ignoreDirs
}

func GetMarkdown(url, extension, toIgnore string) {
	md := Crawl(url, extension, getIgnoreDirs(toIgnore))

	fname := strings.Split(url, "/")[len(strings.Split(url, "/")) - 1]

	f, err := os.Create(fmt.Sprintf("./results/%s.md", fname))
	if err != nil {
		panic(err)
	}

	PrintResults(f, md)
}
