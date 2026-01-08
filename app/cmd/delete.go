package cmd

import (
	"encoding/xml"
	"fmt"
	"os"
)

func RunDeleteItem(itemNumber int) {

	var feed RSS

	data, err := os.ReadFile("feed.xml")
	if err != nil {
		panic(err)
	}

	err = xml.Unmarshal(data, &feed)
	if err != nil {
		panic(err)
	}

	if itemNumber < 1 || itemNumber > len(feed.Channel.Items) {
		println("Invalid item number")
		return
	}

	titleToDelete := feed.Channel.Items[itemNumber-1].Title

	feed.Channel.Items = append(feed.Channel.Items[:itemNumber-1], feed.Channel.Items[itemNumber:]...)

	output, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		panic(err)
	}

	err = os.WriteFile("feed.xml", output, 0644)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Item %s deleted successfully\n", titleToDelete)

}
