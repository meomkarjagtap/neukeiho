package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"
)

func runUpdate() {
	fmt.Println("[neukeiho] checking for latest version...")

	client := &http.Client{Timeout: 15 * time.Second}

	req, _ := http.NewRequest("GET",
		fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo), nil)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to check latest release: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse release info: %v\n", err)
		os.Exit(1)
	}

	if release.TagName == version {
		fmt.Printf("✅ already on latest version %s\n", version)
		return
	}

	fmt.Printf("[neukeiho] new version available: %s (current: %s)\n", release.TagName, version)

	// determine arch suffix
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "arm64"
	case "arm":
		arch = "arm"
	default:
		fmt.Fprintf(os.Stderr, "unsupported arch: %s\n", arch)
		os.Exit(1)
	}

	assetName := fmt.Sprintf("neukeiho-%s-%s", release.TagName, arch)

	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		fmt.Fprintf(os.Stderr, "no binary found for arch %s in release %s\n", arch, release.TagName)
		os.Exit(1)
	}

	fmt.Printf("[neukeiho] downloading %s...\n", assetName)

	binResp, err := client.Get(downloadURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "download failed: %v\n", err)
		os.Exit(1)
	}
	defer binResp.Body.Close()

	// write to temp file
	tmpFile := "/tmp/neukeiho-update"
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp file: %v\n", err)
		os.Exit(1)
	}
	if _, err := io.Copy(f, binResp.Body); err != nil {
		f.Close()
		fmt.Fprintf(os.Stderr, "failed to write binary: %v\n", err)
		os.Exit(1)
	}
	f.Close()

	// find current binary path
	currentBin, err := os.Executable()
	if err != nil {
		currentBin = "/usr/bin/neukeiho"
	}

	// try rename first, fallback to copy
	if err := os.Rename(tmpFile, currentBin); err != nil {
		if err := copyFile(tmpFile, currentBin); err != nil {
			fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
			os.Exit(1)
		}
		os.Remove(tmpFile)
	}

	fmt.Printf("✅ updated to %s — restart neukeiho to apply:\n", release.TagName)
	fmt.Println("   sudo pkill -f 'neukeiho start'")
	fmt.Println("   sudo nohup neukeiho start > /var/log/neukeiho/neukeiho.log 2>&1 &")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
