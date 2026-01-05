package feed

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"
)

const wordpressTVFeedURL = "https://wordpress.tv/feed/"

var (
	iframePattern       = regexp.MustCompile(`(?is)<iframe\b[^>]*>.*?</iframe>`)
	iframeWidthPattern  = regexp.MustCompile(`(?i)\swidth\s*=\s*(?:"[^"]*"|'[^']*'|[^'"\s>]+)`)
	iframeHeightPattern = regexp.MustCompile(`(?i)\sheight\s*=\s*(?:"[^"]*"|'[^']*'|[^'"\s>]+)`)
	iframeAllowPattern  = regexp.MustCompile(`(?i)\sallow\s*=\s*(?:"[^"]*"|'[^']*'|[^'"\s>]+)`)
	anchorBlockPattern  = regexp.MustCompile(`(?is)<a\b[^>]*>.*?</a>`)
	anchorTagPattern    = regexp.MustCompile(`(?is)</?a\b[^>]*>`)
)

type wordPressTVFeed struct {
	Channel wordPressTVChannel `xml:"channel"`
}

type wordPressTVChannel struct {
	Items []wordPressTVItem `xml:"item"`
}

type wordPressTVItem struct {
	Title          string   `xml:"title"`
	Link           string   `xml:"link"`
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
	content := buildWordPressTVContent(item.Title, item.Description, item.ContentEncoded)
	return Item{
		Title:      item.Title,
		Link:       item.Link,
		PubDate:    item.PubDate,
		Content:    content,
		Categories: item.Categories,
	}, nil
}

func buildWordPressTVContent(title, description, encoded string) string {
	title = strings.TrimSpace(title)
	description = stripAnchorTags(strings.TrimSpace(description))
	encoded = strings.TrimSpace(encoded)

	header := ""
	if title != "" {
		header = fmt.Sprintf("<p><strong>%s</strong></p>", title)
	}
	if description != "" {
		header += fmt.Sprintf("<p>%s</p>", description)
	}

	if encoded == "" {
		return header
	}

	normalized := normalizeFirstIframe(encoded)
	return header + stripAnchorTags(normalized)
}

func normalizeFirstIframe(content string) string {
	match := iframePattern.FindString(content)
	if strings.TrimSpace(match) == "" {
		return content
	}
	normalized := normalizeIframe(match)
	return strings.Replace(content, match, normalized, 1)
}

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

	if !strings.Contains(openTag, "<iframe") {
		return value
	}
	openTag = setAttr(openTag, "width", "100%", iframeWidthPattern)
	openTag = setAttr(openTag, "height", "auto", iframeHeightPattern)
	openTag = setAttr(openTag, "allow", "autoplay; fullscreen; encrypted-media", iframeAllowPattern)
	return openTag + rest
}

func setAttr(openTag, name, value string, pattern *regexp.Regexp) string {
	attr := fmt.Sprintf(` %s="%s"`, name, value)
	if pattern.MatchString(openTag) {
		return pattern.ReplaceAllString(openTag, attr)
	}
	return strings.TrimSpace(openTag) + attr
}

func stripAnchorTags(content string) string {
	if content == "" {
		return content
	}
	withoutBlocks := anchorBlockPattern.ReplaceAllString(content, "")
	return anchorTagPattern.ReplaceAllString(withoutBlocks, "")
}
