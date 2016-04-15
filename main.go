package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/northbright/pathhelper"
)

const (
	redisLabsHomeURL    string = "https://redislabs.com"
	ebookHomeURL        string = "https://redislabs.com/ebook/redis-in-action"
	tocBeginTag         string = `<div id="sidebar-toc">`
	tocEndTag           string = `<div id="main-content-holder"`
	tocPattern          string = `<a value="(?P<value>\d*?)" href="(?P<link>.*?)">(<span style="display: none">\d*?</span>)?(?P<title>.*?)</a>`
	pageContentBeginTag string = `<div id="page-content-main">`
	pageContentEndTag   string = `<!-- id="page-content-main" -->`
	riaJsURL            string = "https://redislabs.com/ebook/redis-in-action/foreword"
	riaJsBeginTag       string = `<script type="text/javascript">
//Navigation`
	riaJsEndTag string = `</div>

<div id="ubiquitous-footer">`
)

var (
	levelPatterns map[int]string = map[int]string{
		2: `^(Chapter\s\d{1,2}:\s)|((A|B)\.\d{1,2})\s`,
		3: `^(\d{1,2}\.\d{1,2}\s)|((A|B)\.\d{1,2}\.\d{1,2})\s`,
		4: `^\d{1,2}\.\d{1,2}\.\d{1,2}\s`,
	}

	dirs map[string]string = map[string]string{
		"out": "./ria-ebook",
		"js":  "./ria-ebook/js",
		"css": "./ria-ebook/css",
		"img": "./ria-ebook/img",
	}
)

type tocEntry struct {
	title string
	link  string
	value int
	level int
}

type toc []tocEntry

// Len() is part of sort.Interface.
func (t toc) Len() int {
	return len(t)
}

// Swap() is part of sort.Interface.
func (t toc) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

// Less() is part of sort.Interface.
func (t toc) Less(i, j int) bool {
	return t[i].value > t[j].value
}

func (t toc) updateTocText(tocText string) (newTocText string) {
	newTocText = tocText

	for _, v := range t {
		newLink := fmt.Sprintf("./03%d.html", v.value)
		//fmt.Printf("old: %v\nnew: %v\n", v.link, newLink)
		newTocText = strings.Replace(newTocText, v.link, newLink, -1)
	}

	return newTocText
}

func (t toc) downloadPages(outDir string) (err error) {
	tocText := ""
	newTocText := ""
	for _, v := range t {
		link := redisLabsHomeURL + v.link
		s, err := getPageContent(link)
		if err != nil {
			return err
		}

		if newTocText == "" {
			if tocText, err = getTocText(link); err != nil {
				fmt.Printf("getTocText(%v) error: %v\n", link, err)
				return err
			}
			newTocText = t.updateTocText(tocText)
		}

		s = strings.Replace(s, tocText, newTocText, -1)
		f := path.Join(outDir, fmt.Sprintf("%03d.html", v.value))
		fmt.Printf("link: %v\nf: %v, value=%d, len(s)=%v\n", v.link, f, v.value, len(s))
		if err = ioutil.WriteFile(f, []byte(s), 0755); err != nil {
			return err
		}
	}
	return nil
}

func getContent(contentUrl, beginTag, endTag string) (content string, err error) {
	res, err := http.Get(contentUrl)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	s := string(body)

	beginIndex := strings.Index(s, beginTag)
	endIndex := strings.Index(s, endTag)
	if beginIndex == -1 || endIndex == -1 || beginIndex >= endIndex {
		return "", errors.New(fmt.Sprintf("Can't find content in contentUrl: %v\n", contentUrl))
	}

	s = s[beginIndex:endIndex]
	return s, nil
}

func getRiaJS() (jsText string, err error) {
	return getContent(riaJsURL, riaJsBeginTag, riaJsEndTag)
}

func getPageContent(pageUrl string) (pageContent string, err error) {
	return getContent(pageUrl, pageContentBeginTag, pageContentEndTag)
}

func getTocText(pageUrl string) (tocText string, err error) {
	return getContent(pageUrl, tocBeginTag, tocEndTag)
}

func parseTocText(tocText string) (t toc, err error) {
	re := regexp.MustCompile(tocPattern)
	matches := re.FindAllStringSubmatch(tocText, -1)
	//fmt.Printf("matches = %v\n", matches)

	t = toc{}
	for _, m := range matches {
		if len(m) != 5 {
			fmt.Println("len(m) != 4, = %v\n", len(m))
		}
		value, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			fmt.Printf("strconv.ParseUint(%v, 10, 64) error: %v\n", m[1], err)
			return t, err
		}
		title := m[4]
		link := m[2]
		level := 1
		for k, p := range levelPatterns {
			re := regexp.MustCompile(p)
			if re.MatchString(title) {
				level = k
				break
			}
		}
		//fmt.Printf("level %d: %v\n", level, title)
		entry := tocEntry{title, link, int(value), level}
		t = append(t, entry)
	}

	// Sort by value(desc)
	sort.Sort(t)

	return t, nil

}

func getToc(pageUrl string) (t toc, err error) {
	s, err := getTocText(pageUrl)
	if err != nil {
		return nil, err
	}
	return parseTocText(s)
}

func main() {
	dirs, _ := pathhelper.GetAbsPaths(dirs)

	if err := pathhelper.CreateDirs(dirs, 0755); err != nil {
		fmt.Printf("CreateDirs error: %v\n", err)
		return
	}

	t, err := getToc(ebookHomeURL)
	if err != nil {
		fmt.Printf("getToc() error: %v\n", err)
		return
	}
	fmt.Printf("getToc() OK. TOC = %v\n", t)

	pageContent, err := getPageContent(ebookHomeURL)
	if err != nil {
		fmt.Printf("getPageContent(%v) error: %v\n", ebookHomeURL, err)
		return
	}
	fmt.Printf("getPageCotent() OK. Page content: %v\n", pageContent)

	riaJS, err := getRiaJS()
	if err != nil {
		fmt.Printf("getRiaJS() error: %v\n", err)
		return
	}
	fmt.Printf("getRiaJS() OK. riaJS: %v\n", riaJS)

	if err = t.downloadPages(dirs["out"]); err != nil {
		fmt.Printf("t.downloadPages() error: %v\n", err)
		return
	}
}
