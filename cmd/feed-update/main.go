package main

import (
	"context"
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

func main() {
	root, err := os.Getwd()
	if err != nil {
		fatal(err)
	}

	dataDir := filepath.Join(root, "data")
	sitePath := filepath.Join(dataDir, "site.json")
	entriesPath := filepath.Join(dataDir, "entries.json")
	statePath := filepath.Join(dataDir, "state.json")
	outputPath := filepath.Join(root, "feed.xml")

	site := readJSON[Site](sitePath, Site{
		Title:       "Wapuugotchi RSS",
		Link:        "",
		Description: "",
	})
	state := readJSON[State](statePath, State{})
	entries := readJSON[[]Entry](entriesPath, []Entry{})

	latest, err := fetchLatestWordPress()
	if err != nil {
		fatal(err)
	}

	if latest.Title != "" && latest.Title != state.WordPressLatestTitle {
		state.WordPressLatestTitle = latest.Title
		state.WordPressLatestLink = latest.Link
		state.WordPressLatestPubDate = latest.PubDate
		entries = append(entries, Entry{
			ID:        pickEntryID(latest),
			Title:     latest.Title,
			Link:      latest.Link,
			Content:   latest.Description,
			CreatedAt: pickEntryTime(latest),
		})
	}

	writeJSON(statePath, state)
	writeJSON(entriesPath, entries)

	if err := buildFeed(site, entries, outputPath); err != nil {
		fatal(err)
	}
}

func fetchLatestWordPress() (WordPressItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://wordpress.org/news/category/releases/feed/", nil)
	if err != nil {
		return WordPressItem{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return WordPressItem{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return WordPressItem{}, fmt.Errorf("wordpress api status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return WordPressItem{}, err
	}

	var payload WordPressFeed
	if err := xml.Unmarshal(body, &payload); err != nil {
		return WordPressItem{}, err
	}
	if len(payload.Channel.Items) == 0 {
		return WordPressItem{}, nil
	}
	return payload.Channel.Items[0], nil
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

	output, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer output.Close()

	if _, err := output.WriteString(xml.Header); err != nil {
		return err
	}

	enc := xml.NewEncoder(output)
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
	if strings.TrimSpace(item.GUID) != "" {
		return strings.TrimSpace(item.GUID)
	}
	return fmt.Sprintf("wordpress-%d", time.Now().Unix())
}

func pickEntryTime(item WordPressItem) string {
	parsed, err := parsePubDate(item.PubDate)
	if err != nil {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return parsed.UTC().Format(time.RFC3339)
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

func readJSON[T any](path string, fallback T) T {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return fallback
	}
	return value
}

func writeJSON(path string, value any) {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(path, payload, 0644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
