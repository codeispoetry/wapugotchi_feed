package feed

import (
	"encoding/xml"
	"strings"
)

const wordpressComFeedURL = "https://wordpress.com/blog/feed/"

type wordPressComFeed struct {
	Channel wordPressComChannel `xml:"channel"`
}

type wordPressComChannel struct {
	Items []wordPressComItem `xml:"item"`
}

type wordPressComItem struct {
	Title          string   `xml:"title"`
	Link           string   `xml:"link"`
	GUID           string   `xml:"guid"`
	PubDate        string   `xml:"pubDate"`
	Description    string   `xml:"description"`
	ContentEncoded string   `xml:"encoded"`
	Categories     []string `xml:"category"`
}

func LatestWordPressComBlog(fetch func(url, source string) ([]byte, error)) (Item, error) {
	body, err := fetch(wordpressComFeedURL, "wordpress com")
	if err != nil {
		return Item{}, err
	}

	var feed wordPressComFeed
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
		Description: item.Description,
		Categories:  item.Categories,
	}, nil
}

func pickEncodedOrDescription(encoded, description string) string {
	encoded = strings.TrimSpace(encoded)
	if encoded != "" {
		return encoded
	}
	return strings.TrimSpace(description)
}
