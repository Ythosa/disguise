package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
	for _, e := range elements {
		match, err := regexp.Match(pattern, []byte(e))
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

	var fhref string
	var fname   string
	var dirname string
	var dirhref string

	matchFile := regexp.MustCompile(fmt.Sprintf(".+/blob/.+%s$", extension))
	matchDir := regexp.MustCompile(`.+/tree/.+`)

	for _, a := range n.Attr {
		switch a.Key {
		case "href":
			fhref = "https://github.com" + a.Val
			fname = n.FirstChild.Data

			pathArray := strings.Split(fhref, "/")
			if matchDir.Match([]byte(fhref)) {
				isDir = true
				dirname = strings.Join(pathArray[5:len(pathArray)-1], "/")
			} else if matchFile.Match([]byte(fhref)) {
				isTrackedFile = true
				dirname = strings.Join(pathArray[7:len(pathArray)-1], "/")
			}

			if isDir || isTrackedFile {
				dirhref = strings.Join(pathArray[:len(pathArray)-1], "/")
			}
		case "class":
			if a.Val == "js-navigation-open link-gray-dark" {
				hasRightStyles = true
			}
		}
	}

	if IsContains(ignoreDirs, dirname) {
		return nil
	}

	if hasRightStyles && isTrackedFile {
		return MDFile{
			href:    fhref,
			name:    fname[:len(fname)-len(extension)],
			dir: MDDir{
				href: dirhref,
				name: dirname,
			},
		}
	}

	if hasRightStyles && isDir {
		return MDDir{
			href: dirhref,
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
		log.Fatalf("getting %s by HTML: %v", url, err)
	}

	doc, err := html.Parse(res.Body)
	if err != nil {
		log.Fatalf("analise %s by HTML: %v", url, err)
	}

	files := make([]Element, 0)
	visitNode := func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			files = append(files, CheckLink(n, extension, ignoreDirs))
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

var url = flag.String("url", "", "Which repository should have documentation.")
var extension = flag.String("ext", "", "Which files should have documentation")
var toIgnore = flag.String("ignore", "", "Which dirs shouldn't have documentation.")

func CheckRepositoryURL(url string) (bool, error) {
	match, err := regexp.Match(`^https:\/\/github.com/.*$`, []byte(url))
	if err != nil {
		return false, err
	}
	return match, nil
}

func CheckExtension(ext string) (bool, error) {
	match, err := regexp.Match(`^\.\S*$`, []byte(ext))
	if err != nil {
		return false, err
	}
	return match, nil
}

func CheckInputData(url, ext string) (bool, error) {
	isURL, err := CheckRepositoryURL(url)
	if err != nil {
		return false, err
	}

	isExt, err := CheckExtension(ext)
	if err != nil {
		return false, err
	}

	return isURL && isExt, nil
}

/* Example of starting program
	<./cli_path> [options] --url "<repository_name>" --ext "<files_extension>"
	./disguise --ignore "Platform.Setters.Tests" --url https://github.com/linksplatform/Setters/ --ext ".cs"
*/
func main() {
	flag.Parse()

	correct, err := CheckInputData(*url, *extension)
	if err != nil {
		panic(err)
	}
	if !correct {
		panic("Error. Enter correct data for CLI look like  " +
			"<./cli_path> [options] --url \"<repository_name>\" --ext \"<files_extension>\".\n" +
			"Options could be: \n\t" +
			"--ignore \"<some_dir_name_in_repo>\"")
	}

	var ignoreDirs = strings.Split(*toIgnore, " ")
	md := Crawl(*url, *extension, ignoreDirs)

	fname := strings.Split(*url, "/")[len(strings.Split(*url, "/")) - 1]
	f, err := os.Create(fmt.Sprintf("%s.md", fname))
	if err != nil {
		panic(err)
	}

	PrintResults(f, md)
}
