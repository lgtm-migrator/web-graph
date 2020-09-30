package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	_ "net/http/pprof"

	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/queue"
	"github.com/jamesjarvis/web-graph/pkg/crawler"
	"github.com/jamesjarvis/web-graph/pkg/queueutils"
	_ "github.com/lib/pq"
)

func main() {
	c := colly.NewCollector(
		colly.UserAgent("WebGraph v0.1 https://github.com/jamesjarvis/web-graph - This bot just follows links ¯\\_(ツ)_/¯"),
	)

	crawlerStorage := crawler.Storage{
		URI:       fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable", os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD"), "database", os.Getenv("POSTGRES_DB")),
		PageTable: "pages_visited",
		LinkTable: "links_visited",
	}

	if err := crawlerStorage.Init(); err != nil {
		log.Fatal(err)
	}

	batchPages, err := crawler.NewPageBatcher(5000, &crawlerStorage)
	if err != nil {
		log.Fatal(err)
	}
	batchLinks := crawler.NewLinkBatcher(500, &crawlerStorage)

	q, _ := queue.New(
		12, // Number of consumer threads
		&queue.InMemoryQueueStorage{MaxSize: 5000},
	)

	c.Limit(&colly.LimitRule{
		DomainGlob: "*",
	})

	// Find and visit all links
	c.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := strings.TrimSpace(e.Attr("href"))
		u, err := url.Parse(link)
		if err != nil {
			// log.Printf("ERROR: bad url | %s", link)
			return
		}

		if u.Hostname() == "" {
			u = e.Request.URL.ResolveReference(u)
		}

		if !crawler.ScrapeDaTing(u) {
			return
		}

		if ok, _ := c.HasVisited(e.Request.URL.String()); !ok {
			batchPages.AddPage(&crawler.Page{
				U: e.Request.URL,
			})
		}
		if ok, _ := c.HasVisited(u.String()); !ok {
			batchPages.AddPage(&crawler.Page{
				U: u,
			})
		}

		batchLinks.AddLink(&crawler.Link{
			FromU:    e.Request.URL,
			ToU:      u,
			LinkText: e.Text,
			LinkType: e.Name,
		})

		q.AddURL(link)

	})

	c.OnRequest(func(r *colly.Request) {
		if !crawler.ScrapeDaTing(r.URL) {
			r.Abort()
			return
		}

		batchPages.AddPage(&crawler.Page{
			U: r.URL,
		})
	})

	log.Print("🔥🔥🔥 !!! SCRAPE AWAY !!! 🔥🔥🔥")

	interestingURLs := []string{
		"https://news.ycombinator.com/",
		"https://jamesjarvis.io/",
		"https://www.startups-list.com/",
		"https://www.indiehackers.com/",
		"https://www.cisco.com/",
		"https://thoughtmachine.net/",
		"https://www.bbc.co.uk/",
		"https://www.kent.ac.uk/",
		"https://home.cern/",
		"https://www.nasa.gov/",
		"https://www.engadget.com/",
		"https://www.webdesign-inspiration.com/",
		"https://moz.com/top500",
	}

	for _, url := range interestingURLs {
		q.AddURL(url)
	}

	// This little snippet enabled the go pprof tools
	// http.HandleFunc("/test", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
	// 	rw.Write([]byte("hi"))
	// }))
	// go http.ListenAndServe(":6060", nil)

	qp := queueutils.NewQueuePrinter(q, time.Minute)
	qp.PrintQueueStats()

	// Set up batch workers
	batchLinks.SpawnWorkers(10)
	batchPages.SpawnWorkers(5)

	q.Run(c)

	qp.KillQueuePrinter()
	batchLinks.KillWorkers()
	batchPages.KillWorkers()

	log.Println("Done! 🤯")
}
