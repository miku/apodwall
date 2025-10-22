package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	defaultAPIKey = "DEMO_KEY"
	apodURL       = "https://api.nasa.gov/planetary/apod"
	nasaImagesURL = "https://images-api.nasa.gov/search"
	cacheSubdir   = "apodwall"
)

var cacheDir string
var httpClient *http.Client

var (
	apodFlag      = flag.Bool("a", false, "Display APOD (Astronomy Picture of the Day) image URL")
	nasaFlag      = flag.Bool("n", false, "Display random NASA image URL")
	wallpaperFlag = flag.Bool("w", false, "Set the image as wallpaper (downloads and caches the image)")
	query         = flag.String("q", "sun", "Search query for NASA images")
	timeout       = flag.Duration("T", 30*time.Second, "HTTP request timeout")
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
	flag.Parse()
	httpClient = &http.Client{
		Timeout: *timeout,
	}
	if err := initCacheDir(); err != nil {
		log.Fatal("could not create cache dir")
	}
	apiKey := os.Getenv("DATA_GOV_API_KEY")
	if apiKey == "" {
		apiKey = defaultAPIKey
	}
	switch {
	case *apodFlag:
		if err := fetchAPOD(apiKey, *wallpaperFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching APOD: %v\n", err)
			os.Exit(1)
		}
	case *nasaFlag:
		if err := fetchNASAImage(*query, *wallpaperFlag); err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching NASA image: %v\n", err)
			os.Exit(1)
		}
	default:
		flag.Usage()
		os.Exit(1)
	}
}

// initCacheDir initializes the cache directory using XDG spec
func initCacheDir() error {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		cacheHome = filepath.Join(homeDir, ".cache")
	}
	cacheDir = filepath.Join(cacheHome, cacheSubdir)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}
	return nil
}

// fetchAPOD fetches and displays a random APOD image URL
func fetchAPOD(apiKey string, setWallpaper bool) error {
	var (
		startDate  = time.Date(1995, 6, 16, 0, 0, 0, 0, time.UTC)
		endDate    = time.Now()
		daysDiff   = int(endDate.Sub(startDate).Hours() / 24)
		randomDays = rand.Intn(daysDiff)
		randomDate = startDate.AddDate(0, 0, randomDays)
		dateStr    = randomDate.Format("2006-01-02")
		url        = fmt.Sprintf("%s?api_key=%s&date=%s", apodURL, apiKey, dateStr)
		cacheKey   = fmt.Sprintf("apod_%s.json", dateStr)
		cachePath  = filepath.Join(cacheDir, cacheKey)
		apod       APOD
	)
	if cachedData, err := os.ReadFile(cachePath); err == nil {
		if err := json.Unmarshal(cachedData, &apod); err != nil {
			if err := fetchAndCacheAPOD(url, cachePath, &apod); err != nil {
				return err
			}
		}
	} else {
		if err := fetchAndCacheAPOD(url, cachePath, &apod); err != nil {
			return err
		}
	}
	if apod.MediaType != "image" {
		return fmt.Errorf("APOD for %s is not an image (type: %s)", dateStr, apod.MediaType)
	}
	imageURL := apod.URL
	if apod.HDURL != "" {
		imageURL = apod.HDURL
	}
	fmt.Fprintln(os.Stderr, imageURL)
	if setWallpaper {
		imagePath, err := downloadAndCacheImage(imageURL)
		if err != nil {
			return fmt.Errorf("failed to download image: %w", err)
		}
		if err := setWallpaperImage(imagePath); err != nil {
			return fmt.Errorf("failed to set wallpaper: %w", err)
		}
	}
	return nil
}

// fetchAndCacheAPOD fetches APOD data and caches it
func fetchAndCacheAPOD(url, cachePath string, apod *APOD) error {
	resp, err := httpClient.Get(url)
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
	if err := json.Unmarshal(body, apod); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}
	if err := os.WriteFile(cachePath, body, 0644); err != nil {
		log.Printf("warning: failed to cache response: %v\n", err)
	}
	return nil
}

// fetchNASAImage fetches and displays a random NASA image URL
func fetchNASAImage(query string, setWallpaper bool) error {
	url := fmt.Sprintf("%s?media_type=image&q=%s", nasaImagesURL, query)
	resp, err := httpClient.Get(url)
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
	var (
		randomIdx = rand.Intn(len(items))
		item      = items[randomIdx]
	)
	collResp, err := httpClient.Get(item.Href)
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
	imageURL := imageURLs[0]
	fmt.Fprintf(os.Stderr, "%s\n", imageURL)
	if setWallpaper {
		imagePath, err := downloadAndCacheImage(imageURL)
		if err != nil {
			return fmt.Errorf("failed to download image: %w", err)
		}
		if err := setWallpaperImage(imagePath); err != nil {
			return fmt.Errorf("failed to set wallpaper: %w", err)
		}
	}
	return nil
}

// downloadAndCacheImage downloads an image and caches it locally
func downloadAndCacheImage(imageURL string) (string, error) {
	var (
		hash = sha256.Sum256([]byte(imageURL))
		ext  = filepath.Ext(imageURL)
	)
	if ext == "" {
		ext = ".jpg"
	}
	var (
		filename  = fmt.Sprintf("image_%x%s", hash[:8], ext)
		cachePath = filepath.Join(cacheDir, filename)
	)
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}
	resp, err := httpClient.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download image, status: %d", resp.StatusCode)
	}
	outFile, err := os.Create(cachePath)
	if err != nil {
		return "", fmt.Errorf("failed to create cache file: %w", err)
	}
	defer outFile.Close()
	if _, err := io.Copy(outFile, resp.Body); err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}
	return cachePath, nil
}

// setWallpaperImage sets the wallpaper to the given image path
func setWallpaperImage(imagePath string) error {
	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}
	switch runtime.GOOS {
	case "linux":
		if err := tryGnome(absPath); err == nil {
			return nil
		}
		if err := tryKDE(absPath); err == nil {
			return nil
		}
		if err := tryXFCE(absPath); err == nil {
			return nil
		}
		if err := tryFeh(absPath); err == nil {
			return nil
		}
		return fmt.Errorf("no supported desktop environment found")
	case "darwin":
		var (
			script = fmt.Sprintf(`tell application "Finder" to set desktop picture to POSIX file "%s"`, absPath)
			cmd    = exec.Command("osascript", "-e", script)
		)
		return cmd.Run()
	case "windows":
		return fmt.Errorf("not implemented")
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
}

// tryGnome attempts to set wallpaper using GNOME gsettings
func tryGnome(imagePath string) error {
	cmd := exec.Command("gsettings", "set", "org.gnome.desktop.background", "picture-uri", "file://"+imagePath)
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("gsettings", "set", "org.gnome.desktop.background", "picture-uri-dark", "file://"+imagePath)
	return cmd.Run()
}

// tryKDE attempts to set wallpaper using KDE's qdbus
func tryKDE(imagePath string) error {
	script := fmt.Sprintf(`
var allDesktops = desktops();
for (i=0;i<allDesktops.length;i++) {
	d = allDesktops[i];
	d.wallpaperPlugin = "org.kde.image";
	d.currentConfigGroup = Array("Wallpaper", "org.kde.image", "General");
	d.writeConfig("Image", "file://%s");
}
`, imagePath)
	cmd := exec.Command("qdbus", "org.kde.plasmashell", "/PlasmaShell", "org.kde.PlasmaShell.evaluateScript", script)
	return cmd.Run()
}

// tryXFCE attempts to set wallpaper using XFCE's xfconf-query
func tryXFCE(imagePath string) error {
	cmd := exec.Command("xfconf-query", "-c", "xfce4-desktop", "-p", "/backdrop/screen0/monitor0/workspace0/last-image", "-s", imagePath)
	return cmd.Run()
}

// tryFeh attempts to set wallpaper using feh (fallback for many WMs)
func tryFeh(imagePath string) error {
	cmd := exec.Command("feh", "--bg-scale", imagePath)
	return cmd.Run()
}
