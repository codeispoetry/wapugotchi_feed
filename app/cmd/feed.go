package cmd

import (
	"crypto/md5"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const wordpressFeedURL = "https://wordpress.org/news/category/releases/feed/"

type Site struct {
	Title       string `json:"title"`
	Link        string `json:"link"`
	Description string `json:"description"`
}

type Entry struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Link      string `json:"link"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type State struct {
	WordPressLatestTitle   string `json:"wordpress_latest_title"`
	WordPressLatestLink    string `json:"wordpress_latest_link"`
	WordPressLatestPubDate string `json:"wordpress_latest_pub_date"`
}

type WordPressFeed struct {
	Channel WordPressChannel `xml:"channel"`
}

type WordPressChannel struct {
	Items []WordPressItem `xml:"item"`
}

type WordPressItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

type RSS struct {
	XMLName xml.Name `xml:"rss"`
	Version string   `xml:"version,attr"`
	Channel Channel  `xml:"channel"`
}

type Channel struct {
	Title         string `xml:"title"`
	Link          string `xml:"link"`
	Description   string `xml:"description"`
	LastBuildDate string `xml:"lastBuildDate,omitempty"`
	Items         []Item `xml:"item"`
}

type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        GUID   `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

type GUID struct {
	IsPermaLink string `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

func RunFeedUpdate() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}

	paths := feedPaths(root)
	site := Site{Title: "Wapuugotchi RSS"}
	state := State{}
	entries := []Entry{}

	readJSON(paths.site, &site)
	readJSON(paths.state, &state)
	readJSON(paths.entries, &entries)

	latest, err := getLatestWordPress()
	if err != nil {
		return err
	}

	if latest.Title == "" || latest.Title == state.WordPressLatestTitle {
		fmt.Println("no update detected")
		return nil
	}

	text := strings.TrimSpace(latest.Description)
	if text != "" {
		translated, err := TransformTextByAi(text)
		if err != nil {
			fmt.Fprintln(os.Stderr, "translation failed:", err)
		} else {
			text = translated
		}
	}

	state.WordPressLatestTitle = latest.Title
	state.WordPressLatestLink = latest.Link
	state.WordPressLatestPubDate = latest.PubDate
	createdAt := pickEntryTime(latest)
	id := pickEntryID(latest)
	if !idExists(entries, id) {
		entries = append(entries, Entry{
			ID:        id,
			Title:     latest.Title,
			Link:      latest.Link,
			Content:   text,
			CreatedAt: createdAt,
		})
	}

	writeJSON(paths.state, state)
	writeJSON(paths.entries, entries)

	if err := buildFeed(site, entries, paths.feed); err != nil {
		return err
	}

	fmt.Println("update detected")
	return nil
}

type paths struct {
	site    string
	state   string
	entries string
	feed    string
}

func feedPaths(root string) paths {
	dataDir := filepath.Join(root, "data")
	return paths{
		site:    filepath.Join(dataDir, "site.json"),
		state:   filepath.Join(dataDir, "state.json"),
		entries: filepath.Join(dataDir, "entries.json"),
		feed:    filepath.Join(root, "feed.xml"),
	}
}

func getLatestWordPress() (WordPressItem, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	var body []byte
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest(http.MethodGet, wordpressFeedURL, nil)
		if err != nil {
			return WordPressItem{}, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.7")

		resp, err := client.Do(req)
		if err != nil {
			return WordPressItem{}, err
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			time.Sleep(2 * time.Second)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return WordPressItem{}, fmt.Errorf("wordpress api status: %s", resp.Status)
		}

		body, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return WordPressItem{}, err
		}
		break
	}

	var feed WordPressFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return WordPressItem{}, err
	}
	if len(feed.Channel.Items) == 0 {
		return WordPressItem{}, nil
	}
	return feed.Channel.Items[0], nil
}

func buildFeed(site Site, entries []Entry, outputPath string) error {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt > entries[j].CreatedAt
	})

	channel := Channel{
		Title:       site.Title,
		Link:        site.Link,
		Description: site.Description,
	}

	if len(entries) > 0 {
		last, err := parseTime(entries[0].CreatedAt)
		if err == nil {
			channel.LastBuildDate = last.UTC().Format(time.RFC1123Z)
		}
	}

	for _, entry := range entries {
		createdAt, err := parseTime(entry.CreatedAt)
		if err != nil {
			continue
		}
		channel.Items = append(channel.Items, Item{
			Title:       entry.Title,
			Link:        entry.Link,
			GUID:        GUID{IsPermaLink: "false", Value: entry.ID},
			PubDate:     createdAt.UTC().Format(time.RFC1123Z),
			Description: entry.Content,
		})
	}

	rss := RSS{
		Version: "2.0",
		Channel: channel,
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.WriteString(xml.Header); err != nil {
		return err
	}

	enc := xml.NewEncoder(file)
	enc.Indent("", "  ")
	return enc.Encode(rss)
}

func parseTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "Z") {
		return time.Parse(time.RFC3339, value)
	}
	return time.Parse(time.RFC3339, value)
}

func pickEntryID(item WordPressItem) string {
	return hashPubDate(item.PubDate)
}

func pickEntryTime(item WordPressItem) string {
	parsed, err := parsePubDate(item.PubDate)
	if err != nil {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return parsed.UTC().Format(time.RFC3339)
}

func idExists(entries []Entry, id string) bool {
	for _, entry := range entries {
		if entry.ID == id {
			return true
		}
	}
	return false
}

func parsePubDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty pubDate")
	}
	if parsed, err := time.Parse(time.RFC1123Z, value); err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC1123, value)
}

func hashPubDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Sprintf("pubdate-%d", time.Now().UnixNano())
	}
	sum := md5.Sum([]byte(value))
	return fmt.Sprintf("%x", sum)
}

func readJSON(path string, target any) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, target)
}

func writeJSON(path string, value any) {
	file, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer file.Close()

	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
