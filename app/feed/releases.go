package feed

import "encoding/xml"

const releasesFeedURL = "https://wordpress.org/news/category/releases/feed/"

type Item struct {
	Title       string
	Link        string
	GUID        string
	PubDate     string
	Description string
	Categories  []string
}

type wordPressFeed struct {
	Channel wordPressChannel `xml:"channel"`
}

type wordPressChannel struct {
	Items []wordPressItem `xml:"item"`
}

type wordPressItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	GUID        string   `xml:"guid"`
	PubDate     string   `xml:"pubDate"`
	Description string   `xml:"description"`
	Categories  []string `xml:"category"`
}

func LatestReleases(fetch func(url, source string) ([]byte, error)) (Item, error) {
	body, err := fetch(releasesFeedURL, "wordpress releases")
	if err != nil {
		return Item{}, err
	}

	var feed wordPressFeed
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
