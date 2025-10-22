package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"time"
)

const (
	apiKey        = "DEMO_KEY"
	apodURL       = "https://api.nasa.gov/planetary/apod"
	nasaImagesURL = "https://images-api.nasa.gov/search"
)

// APOD represents the Astronomy Picture of the Day
type APOD struct {
	Copyright   string `json:"copyright"`
	Date        string `json:"date"`
	Explanation string `json:"explanation"`
	HDURL       string `json:"hdurl"`
	MediaType   string `json:"media_type"`
	Title       string `json:"title"`
	URL         string `json:"url"`
}

// NASAImageResponse represents the response from NASA Image Library
type NASAImageResponse struct {
	Collection struct {
		Metadata struct {
			TotalHits int `json:"total_hits"`
		} `json:"metadata"`
		Items []struct {
			Href string `json:"href"`
			Data []struct {
				NASAId      string `json:"nasa_id"`
				Title       string `json:"title"`
				Center      string `json:"center"`
				Description string `json:"description"`
				DateCreated string `json:"date_created"`
			} `json:"data"`
		} `json:"items"`
	} `json:"collection"`
}

// NASAImageCollection represents the collection of image URLs
type NASAImageCollection []string

func main() {
	apodFlag := flag.Bool("a", false, "Display APOD (Astronomy Picture of the Day) image URL")
	nasaFlag := flag.Bool("n", false, "Display random NASA image URL")
	query := flag.String("q", "mars", "Search query for NASA images")

	flag.Parse()

	if *apodFlag {
		if err := fetchAPOD(); err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching APOD: %v\n", err)
			os.Exit(1)
		}
	} else if *nasaFlag {
		if err := fetchNASAImage(*query); err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching NASA image: %v\n", err)
			os.Exit(1)
		}
	} else {
		flag.Usage()
		os.Exit(1)
	}
}

// fetchAPOD fetches and displays a random APOD image URL
func fetchAPOD() error {
	// Generate a random date between June 16, 1995 (APOD start) and today
	startDate := time.Date(1995, 6, 16, 0, 0, 0, 0, time.UTC)
	endDate := time.Now()

	daysDiff := int(endDate.Sub(startDate).Hours() / 24)
	randomDays := rand.Intn(daysDiff)
	randomDate := startDate.AddDate(0, 0, randomDays)

	dateStr := randomDate.Format("2006-01-02")

	url := fmt.Sprintf("%s?api_key=%s&date=%s", apodURL, apiKey, dateStr)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch APOD: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var apod APOD
	if err := json.Unmarshal(body, &apod); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	if apod.MediaType != "image" {
		return fmt.Errorf("APOD for %s is not an image (type: %s)", dateStr, apod.MediaType)
	}

	// Prefer HD URL if available
	if apod.HDURL != "" {
		fmt.Println(apod.HDURL)
	} else {
		fmt.Println(apod.URL)
	}

	return nil
}

// fetchNASAImage fetches and displays a random NASA image URL
func fetchNASAImage(query string) error {
	url := fmt.Sprintf("%s?media_type=image&q=%s", nasaImagesURL, query)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch NASA images: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	var nasaResp NASAImageResponse
	if err := json.Unmarshal(body, &nasaResp); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	totalHits := nasaResp.Collection.Metadata.TotalHits
	if totalHits == 0 {
		return fmt.Errorf("no images found for query: %s", query)
	}

	items := nasaResp.Collection.Items
	if len(items) == 0 {
		return fmt.Errorf("no items in response")
	}

	// Pick a random item
	randomIdx := rand.Intn(len(items))
	item := items[randomIdx]

	// Fetch the collection to get actual image URLs
	collResp, err := http.Get(item.Href)
	if err != nil {
		return fmt.Errorf("failed to fetch image collection: %w", err)
	}
	defer collResp.Body.Close()

	collBody, err := io.ReadAll(collResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read collection: %w", err)
	}

	var imageURLs NASAImageCollection
	if err := json.Unmarshal(collBody, &imageURLs); err != nil {
		return fmt.Errorf("failed to parse collection: %w", err)
	}

	if len(imageURLs) == 0 {
		return fmt.Errorf("no image URLs in collection")
	}

	// Print the first (usually largest) image URL
	fmt.Println(imageURLs[0])

	return nil
}
