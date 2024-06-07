package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gocolly/colly"
	"github.com/schollz/progressbar/v3"
)

var dlink string
var maxjobs int
var downloadFlac bool
var outPutPath string

func main() {
	flag.StringVar(&dlink, "link", "", "Paste the full link of OST from downloads.khinsider.com")
	flag.IntVar(&maxjobs, "j", 1, "Maximum parallel downloads. Use with caution.")
	flag.BoolVar(&downloadFlac, "flac", false, "Download FLAC version of soundtrack")
	flag.StringVar(&outPutPath, "out", "./", "Download path")
	flag.Parse()

	var songLinks []string
	var downloadLinks []string

	c := colly.NewCollector(
		colly.AllowedDomains("downloads.khinsider.com"),
	)

	c.OnHTML("#songlist", func(h *colly.HTMLElement) {
		h.ForEach("tr", func(i int, h *colly.HTMLElement) {
			h.ForEach(".playlistDownloadSong", func(i int, h *colly.HTMLElement) {
				h.ForEach("a", func(i int, h *colly.HTMLElement) {
					songLinks = append(songLinks, "https://downloads.khinsider.com"+h.Attr("href"))
				})
			})
		})
	})

	d := colly.NewCollector(
		colly.AllowedDomains("downloads.khinsider.com"),
		colly.Async(true),
	)

	d.Limit(&colly.LimitRule{
		Parallelism: 2,
		Delay:       3 * time.Second,
	})

	d.OnHTML("#audio", func(h *colly.HTMLElement) {
		link := h.Attr("src")
		if downloadFlac {
			re := regexp.MustCompile(".mp3$")
			link = re.ReplaceAllString(link, ".flac")
		}
		downloadLinks = append(downloadLinks, link)
	})

	err := c.Visit(dlink)
	if err != nil {
		log.Fatal(err)
	}

	c.Wait()

	for _, i := range songLinks {
		d.Visit(i)
	}
	d.Wait()

	var wg sync.WaitGroup

	concJobs := make(chan struct{}, maxjobs)

	for _, i := range downloadLinks {
		wg.Add(1)
		go download(i, &wg, concJobs)
	}
	wg.Wait()

}

func download(link string, wg *sync.WaitGroup, ch chan struct{}) {
	defer time.Sleep(2 * time.Second)
	defer wg.Done()

	i_s := strings.Split(link, "/")
	_, err := os.Stat(path.Clean(outPutPath) + "/" + i_s[len(i_s)-1])
	if err == nil {
		log.Printf("File \"%s\" exist. Skipping...", path.Clean(outPutPath)+"/"+i_s[len(i_s)-1])
		return
	}

	ch <- struct{}{}

	resp, err := http.Get(link)
	if err != nil {
		log.Println(err)
		return
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
		return
	}
	defer resp.Body.Close()

	log.Printf("Downloading: %s", link)
	progr := progressbar.DefaultBytes(resp.ContentLength)
	buf := bufio.NewReader(resp.Body)

	f_name, _ := url.QueryUnescape(i_s[len(i_s)-1])

	_, err = os.Stat(outPutPath)
	if err != nil {
		os.MkdirAll(outPutPath, 0755)
	}

	f, err := os.Create(path.Clean(outPutPath) + "/" + f_name)
	if err != nil {
		log.Println(err)
		return
	}

	_, err = io.Copy(
		io.MultiWriter(
			f, progr,
		),
		buf,
	)
	if err != nil {
		log.Println(err)
		os.Remove(path.Clean(outPutPath) + "/" + f_name)
		return
	}
	<-ch
}
