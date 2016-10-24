package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/northbright/pathhelper"
)

const (
	redisLabsHomeURL       string = "https://redislabs.com"
	ebookHomeURL           string = "https://redislabs.com/ebook/redis-in-action"
	tocBeginTag            string = `<div id="sidebar-toc">`
	tocEndTag              string = `<div id="main-content-holder"`
	tocPattern             string = `<a value="(?P<value>\d*?)" href="(?P<link>.*?)">(<span style="display: none">\d*?</span>)?(?P<title>.*?)</a>`
	pageContentBeginTag    string = `<div id="page-content-main">`
	pageContentEndTag      string = `<!-- id="page-content-main" -->`
	academyContentBeginTag string = `<div id="academy-content">`
	academyContentEndTag   string = `<!-- id="academy-content" -->`
	riaJsURL               string = "https://redislabs.com/ebook/redis-in-action/foreword"
	riaJsBeginTag          string = `<script type="text/javascript">
//Navigation`
	riaJsEndTag string = `</div>

<div id="ubiquitous-footer">`
	imgSrcPattern string = `<img src="(?P<img_src>.*?)">`
	retryCount    int    = 3
)

var (
	// TOC level patterns. Default level is 1(top level).
	levelPatterns = map[int]string{
		2: `^(Chapter\s\d{1,2}:\s)|((A|B)\.\d{1,2})\s`,
		3: `^(\d{1,2}\.\d{1,2}\s)|((A|B)\.\d{1,2}\.\d{1,2})\s`,
		4: `^\d{1,2}\.\d{1,2}\.\d{1,2}\s`,
	}

	// Predefined Dirs.
	dirs = map[string]string{
		"out": "./ria-ebook",
		"js":  "./ria-ebook/js",
		"css": "./ria-ebook/css",
		"img": "./ria-ebook/img",
	}

	// CSS URLs need to be downloaded.
	cssURLs = []string{
		"https://redislabs.com/wp-content/themes/twentyeleven/style.css",
		"https://redislabs.com/wp-content/themes/twentyeleven/redislabs.css",
		"https://redislabs.com/wp-content/themes/twentyeleven/ria.css",
		"https://redislabs.com/wp-content/themes/twentyeleven/css/fancy.css",
		"https://redislabs.com/wp-content/themes/twentyeleven/js/highlight/default.css",
	}
)

// Cached image info.
type cachedImage struct {
	imgSrc   string
	localSrc string
}

// TOC entry info.
type tocEntry struct {
	title string
	link  string
	value int
	level int
}

// TOC
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
	return t[i].value < t[j].value
}

// toHTMLStr() outputs the TOC to HTML string.
func (t toc) toHTMLStr() (htmlStr string) {
	htmlStr = `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8" />
<title>Redis in Action: Table Of Content</title>
</head>
<body>
<h1 style="margin-top:20px; margin-left:30px; font-size:48px;">Redis in Action</h1>
`

	htmlStr += "<div style=\"margin-left:auto; margin-right:auto; margin-top:80px; margin-bottom:40px;\">\n<ul>\n"
	currentLevel := 1
	for _, v := range t {
		if v.level > currentLevel {
			htmlStr += "<ul>\n"
			currentLevel = v.level
		} else if v.level < currentLevel {
			for i := v.level; i < currentLevel; i++ {
				htmlStr += "</ul>\n"
			}
			currentLevel = v.level
		}

		htmlStr += fmt.Sprintf("<li><a href=\"./%03d.html\">%s</a></li>\n", v.value, v.title)
	}
	htmlStr += "</ul>\n</div>\n</body>\n</html>"
	return htmlStr
}

// writeToHTML() generates the HTML file contains the TOC of Redis in Action.
func (t toc) writeToHTML(f string) (err error) {
	s := t.toHTMLStr()
	return ioutil.WriteFile(f, []byte(s), 0755)
}

// downloadFile() downloads the file by given url and file path.
func downloadFile(fileURL string, filePath string) (err error) {
	res, err := http.Get(fileURL)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filePath, data, 0755)

}

// cacheCSSFiles() downlaods the CSS style files by given CSS file URLs.
func cacheCSSFiles(cssURLs []string, cssDir string) (err error) {
	for _, v := range cssURLs {
		src := v
		localFile := path.Join(cssDir, filepath.Base(src))
		if err = downloadFile(src, localFile); err != nil {
			return err
		}
	}
	return nil
}

// cacheImages() downloads the images in page content.
func cacheImages(academyContent, imgDir string) (cachedImgs []cachedImage, err error) {
	cachedImgs = []cachedImage{}
	re := regexp.MustCompile(imgSrcPattern)
	matches := re.FindAllStringSubmatch(academyContent, -1)

	for _, m := range matches {
		imgSrc := m[1]
		realImgSrc := imgSrc

		// Check if imgSrc starts with "/wp-content".
		// e.g.
		// "/wp-content/images/academy/redis-in-action/RIA_fig4-02.svg"
		// update the src to:
		// "https://redislabs.com/wp-content/images/academy/redis-in-action/RIA_fig4-02.svg"
		if strings.HasPrefix(imgSrc, "/wp-content") {
			realImgSrc = fmt.Sprintf("%s%s", redisLabsHomeURL, imgSrc)
		}

		imgFile := path.Join(imgDir, filepath.Base(imgSrc))
		fmt.Printf("imgSrc: %v, imgFile: %v\n", imgSrc, imgFile)
		if err = downloadFile(realImgSrc, imgFile); err != nil {
			return cachedImgs, err
		}

		localSrc := path.Join("./img/", filepath.Base(imgSrc))
		cachedImgs = append(cachedImgs, cachedImage{imgSrc, localSrc})
	}
	return cachedImgs, nil
}

// downloadPages() download all pages according to the TOC of Redis in Action.
func downloadPages(t toc, pageTmplStr, outDir string) (err error) {
	for _, v := range t {
		link := redisLabsHomeURL + v.link
		s := ""
		if s, err = getAcademyContent(link); err != nil {
			for i := 1; i <= retryCount; i++ {
				fmt.Printf("Retry #%d: %v\n", i, v.link)
				if s, err = getAcademyContent(link); err == nil {
					break
				}
				if i == retryCount {
					return fmt.Errorf("Failed to get academy content.")
				}
			}
		}

		// Cache Images
		cachedImgs, err := cacheImages(s, path.Join(outDir, "img"))
		if err != nil {
			return err
		}

		fmt.Printf("cachedImgs: %v\n", cachedImgs)

		for _, v := range cachedImgs {
			s = strings.Replace(s, v.imgSrc, v.localSrc, -1)
		}

		p := path.Join(outDir, fmt.Sprintf("%03d.html", v.value))
		fmt.Printf("link: %v\nf: %v, value=%d, len(s)=%v\n", v.link, p, v.value, len(s))

		f, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			return err
		}
		defer f.Close()

		tmpl, err := template.New("page").Parse(pageTmplStr)
		if err != nil {
			return err
		}

		prev := ""
		if v.value-1 >= 0 {
			prev = fmt.Sprintf("<a href=\"./%03d.html\">Previous</a>", v.value-1)
		} else {
			prev = fmt.Sprintf("<div style=\"color:#808080;\">Previous</div>")
		}

		next := ""
		if v.value+1 <= 189 {
			next = fmt.Sprintf("<a href=\"./%03d.html\">Next</a>", v.value+1)
		} else {
			next = fmt.Sprintf("<div style=\"color:#808080;\">Next</div>")
		}

		data := struct {
			Title       string
			PageContent string
			Prev        string
			Next        string
		}{
			v.title,
			s,
			prev,
			next,
		}

		if err = tmpl.Execute(f, data); err != nil {
			return err
		}
	}
	return nil
}

// getContent() uses regular expression to get the content in HTML body by given content begin and end tags.
func getContent(contentURL, beginTag, endTag string) (content string, err error) {
	res, err := http.Get(contentURL)
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
		return "", fmt.Errorf("Can't find content in contentURL: %v\n", contentURL)
	}

	s = s[beginIndex:endIndex]
	return s, nil
}

// getRiaJS() gets the Javascript object used in Redis in Action ebook.
func getRiaJS() (jsText string, err error) {
	return getContent(riaJsURL, riaJsBeginTag, riaJsEndTag)
}

// getAcademyContent() gets the academy content(each page) of Redis in Action ebook.
func getAcademyContent(pageURL string) (academyContent string, err error) {
	return getContent(pageURL, academyContentBeginTag, academyContentEndTag)
}

// getTocText() gets the TOC text of Redis in Action ebook.
func getTocText(pageURL string) (tocText string, err error) {
	return getContent(pageURL, tocBeginTag, tocEndTag)
}

// parseTocText() parses the TOC text and return TOC struct of Redis in Action.
func parseTocText(tocText string) (t toc, err error) {
	re := regexp.MustCompile(tocPattern)
	matches := re.FindAllStringSubmatch(tocText, -1)

	t = toc{}
	for _, m := range matches {
		if len(m) != 5 {
			fmt.Printf("len(m) != 4, = %v\n", len(m))
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

		entry := tocEntry{title, link, int(value), level}
		t = append(t, entry)
	}

	// Sort by value
	sort.Sort(t)

	return t, nil
}

// getToc gets the TOC of Redis in Action.
func getToc(pageURL string) (t toc, err error) {
	s, err := getTocText(pageURL)
	if err != nil {
		return nil, err
	}
	return parseTocText(s)
}

func main() {
	fmt.Printf("=========================================\nStarting caching Redis in Action ebook.\n\n")
	dirs, _ := pathhelper.GetAbsPaths(dirs)

	if err := pathhelper.CreateDirs(dirs, 0755); err != nil {
		fmt.Printf("CreateDirs error: %v\n", err)
		return
	}

	if err := cacheCSSFiles(cssURLs, dirs["css"]); err != nil {
		fmt.Printf("cacheCSSFiles() error: %v\n", err)
	}

	t, err := getToc(ebookHomeURL)
	if err != nil {
		fmt.Printf("getToc() error: %v\n", err)
		return
	}
	fmt.Printf("getToc() OK. TOC = %v\n", t)

	tocFile := path.Join(dirs["out"], "_toc.html")
	if err = t.writeToHTML(tocFile); err != nil {
		fmt.Printf("writeToHTML(%v): error: %v\n", tocFile, err)
		return
	}

	if err = downloadPages(t, pageTemplateStr, dirs["out"]); err != nil {
		fmt.Printf("downloadPages() error: %v\n", err)
		return
	}

	fmt.Printf("\n\n=========================================\nRedis in Action ebook has been cached in %v successfully.\n", dirs["out"])
}

var pageTemplateStr string = `
<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8" />
<title>{{.Title}}</title>
<link rel="stylesheet" type="text/css" href="./css/style.css" />
<link rel="stylesheet" type="text/css" href="./css/redislabs.css" />
<link rel="stylesheet" type="text/css" href="./css/ria.css" />
<link rel="stylesheet" type="text/css" href="./css/fancy.css" />
<link rel="stylesheet" type="text/css" href="./css/default.css" />
</head>
<body>
<div style="margin-top: 120px">

<div id="page-content-main">
{{.PageContent}}
</div><!-- id="page-content-main" -->

<div style="margin-left: auto; margin-right: auto; margin-top:20px; width:960px; height:100px; font-size:32px">
  <div style="width:33%; float:left; text-align: left">
    {{.Prev}}
  </div>
  <div style="width:33%; float:left; text-align: center">
    <a href="./_toc.html">Table of Content</a>
  </div>
  <div style="width:33%; float: right; text-align: right">
    {{.Next}}
  </div>
</div>

</div>
</body>
</html>
`
