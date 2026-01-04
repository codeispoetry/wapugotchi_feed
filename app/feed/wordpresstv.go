package feed

import (
	"encoding/xml"
	"fmt"
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
		Description: pickContentEncoded(item.ContentEncoded, item.Description),
		Categories:  item.Categories,
	}, nil
}

var iframePattern = regexp.MustCompile(`(?is)<iframe\b[^>]*>.*?</iframe>`)

func pickContentEncoded(encoded, description string) string {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return stripAnchorTags(strings.TrimSpace(description))
	}
	normalized := normalizeFirstIframe(encoded)
	return stripAnchorTags(normalized)
}

var (
	iframeWidthPattern  = regexp.MustCompile(`(?i)\swidth\s*=\s*(?:"[^"]*"|'[^']*'|[^'"\s>]+)`)
	iframeHeightPattern = regexp.MustCompile(`(?i)\sheight\s*=\s*(?:"[^"]*"|'[^']*'|[^'"\s>]+)`)
	anchorTagPattern    = regexp.MustCompile(`(?is)</?a\b[^>]*>`)
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

	if !strings.HasSuffix(openTag, "<iframe") && !strings.Contains(openTag, "<iframe") {
		return value
	}
	openTag = setAttr(openTag, "width", "100%", iframeWidthPattern)
	openTag = setAttr(openTag, "height", "auto", iframeHeightPattern)
	return openTag + rest
}

func setAttr(openTag, name, value string, pattern *regexp.Regexp) string {
	attr := fmt.Sprintf(` %s="%s"`, name, value)
	if pattern.MatchString(openTag) {
		return pattern.ReplaceAllString(openTag, attr)
	}
	return strings.TrimSpace(openTag) + attr
}

func normalizeFirstIframe(content string) string {
	match := iframePattern.FindString(content)
	if strings.TrimSpace(match) == "" {
		return content
	}
	normalized := normalizeIframe(match)
	return strings.Replace(content, match, normalized, 1)
}

func stripAnchorTags(content string) string {
	if content == "" {
		return content
	}
	return anchorTagPattern.ReplaceAllString(content, "")
}
