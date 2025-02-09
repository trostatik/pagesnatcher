package pagesnatcher

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"
)

func getOtherAssets(content string, domains []string, outputDir string) error {
	if len(domains) == 0 {
		return nil
	}

	// Pattern	Meaning
	// https:\/\/	Match the literal https://
	// `(?:%s)	domain name)`
	// [^\s"']*	Match the rest of the URL (excluding spaces and quotes)
	reg := `https:\/\/%s?[^\s"']*`
	srcPattern := fmt.Sprintf(reg, regexp.QuoteMeta(domains[0]))
	for _, domain := range domains[1:] {
		srcPattern += fmt.Sprintf(`|`+reg, regexp.QuoteMeta(domain))
	}
	srcRe := regexp.MustCompile(srcPattern)
	srcMatches := srcRe.FindAllString(string(content), -1)

	// Create a temporary file to store the URLs
	fileName := fmt.Sprintf("urls-%s.txt", time.Now())
	tmpFile, err := os.CreateTemp("", fileName)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name()) // Clean up the temporary file

	// Write the URLs to the temporary file
	for _, url := range srcMatches {
		if _, err := tmpFile.WriteString(url + "\n"); err != nil {
			return fmt.Errorf("failed to write URL to temporary file: %w", err)
		}
	}
	tmpFile.Close()

	wgetCmd := exec.Command(
		"wget2",
		"--timestamping", // Only re-retrieve if newer
		"--quota=120m",
		"--force-directories", // Create directory structure
		"-i", tmpFile.Name(),
	)
	wgetCmd.Dir = outputDir
	wgetCmd.Stdout = os.Stdout
	wgetCmd.Stderr = os.Stderr

	if err := wgetCmd.Run(); err != nil {
		return fmt.Errorf("failed to run wget2: %w", err)
	}

	return nil
}
