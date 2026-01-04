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

	"wapuugotchi/feed/app/feed"
)

type Site struct {
	Title       string `json:"title"`
	Link        string `json:"link"`
	Description string `json:"description"`
}

type Entry struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Link       string   `json:"link"`
	Content    string   `json:"content"`
	CreatedAt  string   `json:"created_at"`
	Categories []string `json:"categories,omitempty"`
}

type State struct {
	Latest map[string]string `json:"latest,omitempty"`
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
	ID          string   `xml:"id"`
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	PubDate     string   `xml:"pubDate"`
	Description string   `xml:"description"`
	Categories  []string `xml:"category,omitempty"`
}

const (
	userAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	acceptHeader = "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.7"
)

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
	if state.Latest == nil {
		state.Latest = map[string]string{}
	}

	updated := false
	var firstErr error
	for _, provider := range feedProviders() {
		added, err := addLatestFromProvider(provider, &entries, &state)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		if added {
			updated = true
		}
	}

	if !updated && firstErr != nil {
		return firstErr
	}
	if !updated {
		fmt.Println("no update detected")
		return nil
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

type feedProvider struct {
	Name      string
	Translate bool
	Fetch     func(fetch func(url, source string) ([]byte, error)) (feed.Item, error)
}

func feedProviders() []feedProvider {
	return []feedProvider{
		{Name: "wordpress-releases", Translate: true, Fetch: feed.LatestReleases},
		{Name: "wordpress-tv", Translate: false, Fetch: feed.LatestWordPressTV},
		{Name: "wordpress-com", Translate: false, Fetch: feed.LatestWordPressComBlog},
	}
}

func addLatestFromProvider(provider feedProvider, entries *[]Entry, state *State) (bool, error) {
	item, err := provider.Fetch(fetchFeed)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(item.Title) == "" {
		return false, nil
	}

	item.Categories = cleanCategories(item.Categories)
	id := pickEntryID(provider.Name, item)
	if idExists(*entries, id) {
		state.Latest[provider.Name] = id
		return false, nil
	}

	text := translateContent(item.Description, provider.Translate)
	*entries = append(*entries, Entry{
		ID:         id,
		Title:      item.Title,
		Link:       item.Link,
		Content:    text,
		CreatedAt:  pickEntryTime(item),
		Categories: item.Categories,
	})

	state.Latest[provider.Name] = id
	return true, nil
}

func translateContent(text string, allow bool) string {
	text = strings.TrimSpace(text)
	if text == "" || !allow || strings.Contains(strings.ToLower(text), "<iframe") {
		return text
	}

	translated, err := TransformTextByAi(text)
	if err != nil {
		fmt.Fprintln(os.Stderr, "translation failed:", err)
		return text
	}
	return translated
}

func fetchFeed(url, source string) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	var body []byte
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", acceptHeader)

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			time.Sleep(2 * time.Second)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, fmt.Errorf("%s api status: %s", source, resp.Status)
		}

		body, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		break
	}

	return body, nil
}

func cleanCategories(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	return result
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
			ID:          entry.ID,
			PubDate:     createdAt.UTC().Format(time.RFC1123Z),
			Description: entry.Content,
			Categories:  entry.Categories,
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
	return time.Parse(time.RFC3339, strings.TrimSpace(value))
}

func pickEntryID(provider string, item feed.Item) string {
	base := strings.TrimSpace(item.PubDate)
	if base == "" {
		base = strings.TrimSpace(item.Link)
	}
	if base == "" {
		base = fmt.Sprintf("%s-%d", provider, time.Now().UnixNano())
	}
	return hashString(provider + "|" + base)
}

func pickEntryTime(item feed.Item) string {
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

func hashString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Sprintf("hash-%d", time.Now().UnixNano())
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
