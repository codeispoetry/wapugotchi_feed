package feed

import (
	"encoding/xml"
	"regexp"
	"strings"
)

const wordpressTVFeedURL = "https://wordpress.tv/feed/"

type wordPressTVFeed struct {
	Channel wordPressTVChannel `xml:"channel"`
}

type wordPressTVChannel struct {
	Items []wordPressTVItem `xml:"item"`
}

type wordPressTVItem struct {
	Title          string   `xml:"title"`
	Link           string   `xml:"link"`
	GUID           string   `xml:"guid"`
	PubDate        string   `xml:"pubDate"`
	Description    string   `xml:"description"`
	ContentEncoded string   `xml:"encoded"`
	Categories     []string `xml:"category"`
}

func LatestWordPressTV(fetch func(url, source string) ([]byte, error)) (Item, error) {
	body, err := fetch(wordpressTVFeedURL, "wordpress tv")
	if err != nil {
		return Item{}, err
	}

	var feed wordPressTVFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return Item{}, err
	}
	if len(feed.Channel.Items) == 0 {
		return Item{}, nil
	}

	item := feed.Channel.Items[0]
	return Item{
		Title:       item.Title,
		Link:        item.Link,
		GUID:        item.GUID,
		PubDate:     item.PubDate,
		Description: extractFirstIframe(item.ContentEncoded),
		Categories:  item.Categories,
	}, nil
}

var iframePattern = regexp.MustCompile(`(?is)<iframe\b[^>]*>.*?</iframe>`)

func extractFirstIframe(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	match := iframePattern.FindString(value)
	return normalizeIframe(strings.TrimSpace(match))
}

var (
	iframeWidthPattern  = regexp.MustCompile(`(?i)\swidth\s*=\s*(['"]?)[^'"\s>]*\1`)
	iframeHeightPattern = regexp.MustCompile(`(?i)\sheight\s*=\s*(['"]?)[^'"\s>]*\1`)
)

func normalizeIframe(value string) string {
	if value == "" {
		return ""
	}
	tagEnd := strings.Index(value, ">")
	if tagEnd == -1 {
		return value
	}
	openTag := value[:tagEnd]
	rest := value[tagEnd:]

	openTag = iframeWidthPattern.ReplaceAllString(openTag, "")
	openTag = iframeHeightPattern.ReplaceAllString(openTag, "")
	openTag = strings.TrimSpace(openTag)
	if !strings.HasSuffix(openTag, "<iframe") && !strings.Contains(openTag, "<iframe") {
		return value
	}
	openTag = openTag + ` width="100%" height="auto"`
	return openTag + rest
}
