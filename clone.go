package pagesnatcher

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"runtime"

	"golang.org/x/net/publicsuffix"
)

func init() {
	// Check if dependencies are installed
	_, err := exec.LookPath("wget2")
	if err != nil {
		panic("wget2 is not installed")
	}
	_, err = exec.LookPath("find")
	if err != nil {
		panic("find is not installed")
	}
	_, err = exec.LookPath("sed")
	if err != nil {
		panic("sed is not installed")
	}
}

type Profile struct {
	Domains        []string
	ReplaceStrings map[string]string
	QueryFilePaths []string
}

type Config struct {
	TargetURL      string
	OutputDir      string
	Clean          *bool
	ZipPath        string
	Domains        []string
	QueryFilePaths []string
	ReplaceStrings map[string]string
}

type Service struct {
	*Config
}

func (c *Config) Apply(profile string) {
	if profile == "none" {
		return
	}
	p, ok := Profiles[profile]
	if !ok {
		panic(fmt.Sprintf("Profile %s not found", profile))
	}

	// Add to config
	c.Domains = append(c.Domains, p.Domains...)
	c.QueryFilePaths = append(c.QueryFilePaths, p.QueryFilePaths...)

	// Add replace strings from profile to config
	if c.ReplaceStrings == nil {
		c.ReplaceStrings = make(map[string]string)
	}
	for find, replace := range p.ReplaceStrings {
		c.ReplaceStrings[find] = replace
	}

}

func NewService(config *Config) *Service {
	config.TargetURL = cleanSourceName(config.TargetURL)

	// Defaults
	if config.Clean == nil {
		clean := false
		config.Clean = &clean
	}
	if config.OutputDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			panic("Could not get working directory")
		}
		config.OutputDir = filepath.Join(wd, config.TargetURL)
	}

	// Add domain
	rootDomain, err := publicsuffix.EffectiveTLDPlusOne(config.TargetURL)
	if err != nil {
		return nil
	}
	config.Domains = append(config.Domains, rootDomain)

	// Setup output directory
	if err := os.MkdirAll(config.OutputDir, os.ModePerm); err != nil {
		panic(err)
	}

	if *config.Clean {
		removeContents(config.OutputDir)
	}

	return &Service{Config: config}
}

func cleanSourceName(rawName string) string {
	// Remove leading "http://" or "https://"
	cleanString := strings.TrimPrefix(rawName, "http://")
	cleanString = strings.TrimPrefix(cleanString, "https://")

	// Remove trailing periods and spaces
	cleanString = strings.TrimRight(cleanString, ". /")

	return cleanString
}

func (s *Service) DownloadSite(outputWriter io.Writer) error {
	log.Println(s)
	source := s.TargetURL

	var restrictFileNames string
	if runtime.GOOS == "windows" {
		restrictFileNames = "windows"
	} else {
		restrictFileNames = "unix"
	}

	wgetCmd := exec.Command(
		"wget2",
		"--recursive",
		"--level=5",          // recursive depth
		"--timestamping",     // Only re-retrieve if newer
		"--page-requisites",  // images, css, etc
		"--adjust-extension", // convert pages to html, eg asp
		"--restrict-file-names="+restrictFileNames,
		"--convert-links",
		"--no-robots",
		"--quota=120m",
		// "--limit-rate=100k",
		"--domains="+strings.Join(s.Domains, ","),
		source,
	)
	wgetCmd.Dir = s.OutputDir
	wgetCmd.Stdout = outputWriter
	wgetCmd.Stderr = outputWriter

	if err := wgetCmd.Start(); err != nil {
		return fmt.Errorf("failed to run wget2: %w", err)
	}

	if err := wgetCmd.Wait(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			ws := exitError.Sys().(syscall.WaitStatus)
			if ws.ExitStatus() == 8 {
				fmt.Println("Ignoring wget exit code 8 (server error)")
			} else {
				return fmt.Errorf("failed to download site: %w", err)
			}
		}
	}

	fmt.Fprintln(outputWriter, "Downloading additional files")
	// Traverse the directory to find dynamically imported files
	err := filepath.WalkDir(s.OutputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Process only JavaScript or MJS files
		if !d.IsDir() && (strings.HasSuffix(d.Name(), ".js") || strings.HasSuffix(d.Name(), ".mjs")) {

			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("could not read file %s: %w", path, err)
			}

			// Extract and download dynamic imports
			if err := processDynamicImports(s.OutputDir, path, string(content)); err != nil {
				return fmt.Errorf("error processing dynamic imports for %s: %w", path, err)
			}

			if err := getOtherAssets(string(content), s.Domains, s.OutputDir); err != nil {
				return fmt.Errorf("error processing other assets from %s: %w", path, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking directory: %w", err)
	}

	fmt.Fprintln(outputWriter, "Fixing references")
	// Update references in .js and .mjs files
	err = filepath.WalkDir(s.OutputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Process only JavaScript or MJS files
		if !d.IsDir() && (strings.HasSuffix(d.Name(), ".js") || strings.HasSuffix(d.Name(), ".mjs")) {
			// Replace https:// references
			for _, domain := range s.Domains {
				if err := findAndReplaceFile(path, "https://"+domain, "./"+domain); err != nil {
					log.Println(err)
					return fmt.Errorf("error processing text replace: %w", err)
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking directory again: %w", err)
	}

	return nil
}

func findAndReplaceFile(filePath, oldText, newText string) error {
	// Read the file contents
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Perform find & replace
	updatedContent := strings.ReplaceAll(string(content), oldText, newText)

	// Write the updated content back to the file
	err = os.WriteFile(filePath, []byte(updatedContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write updated content: %w", err)
	}

	return nil
}

// processDynamicImports extracts and downloads dynamic imports into their original folder
func processDynamicImports(baseDir string, filePath string, content string) error {
	imports, err := extractDynamicImports(filePath, content)
	if err != nil {
		return fmt.Errorf("could not extract imports: %w", err)
	}

	for _, importName := range imports {
		// Resolve the absolute path of the imported file
		originalDir := filepath.Dir(filePath)
		targetPath := filepath.Join(originalDir, importName)

		// Get base
		importPath, err := filepath.Rel(baseDir, filePath)
		if err != nil {
			log.Println("file error", err)
			return err
		}

		// Add file name
		importPath = filepath.Join(filepath.Dir(importPath), importName)

		// Download the file to the target directory
		wgetCmd := exec.Command("wget2",
			"--timestamping",
			"-O",
			targetPath,
			importPath)

		if err := wgetCmd.Run(); err != nil {
			fmt.Printf("failed to download %s: %v\n", importPath, err)
		}
	}
	return nil
}

// extractDynamicImports reads a file and extracts dynamic imports
func extractDynamicImports(filePath string, content string) ([]string, error) {
	var imports []string

	// Regex to find dynamic imports like import("url") or import('./file.mjs')
	importRegex := regexp.MustCompile(`import\s*\(\s*["']([^"']+)["']\s*\)`)

	matches := importRegex.FindAllStringSubmatch(string(content), -1)

	for _, match := range matches {
		if len(match) > 1 {
			log.Println("0 appending", match[1])
			imports = append(imports, match[1])
		}
	}

	return imports, nil
}

func replaceInHtml(source string, old string, new string, dir string) error {
	cmd := exec.Command("find", source, "-name", "*.html",
		"-exec", "sed", "-i",
		fmt.Sprintf("s|%s|%s|g", old, new),
		"{}", ";")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to update %s references: %w", old, err)
	}
	return nil
}

func (s *Service) FixLocalSite() error {
	source := s.TargetURL
	for old, new := range s.ReplaceStrings {
		err := replaceInHtml(source, old, new, s.OutputDir)
		if err != nil {
			return err
		}

	}

	// Move directories
	dir := fmt.Sprintf("%s/%s/*", s.OutputDir, source)
	files, err := filepath.Glob(dir)
	if err != nil {
		return fmt.Errorf("failed to move files: %v", err)
	}

	// Ensure files were found
	if len(files) == 0 {
		return fmt.Errorf("No files found to move")
	}

	// Move each file
	for _, file := range files {
		newPath := filepath.Join(s.OutputDir, filepath.Base(file))
		if err := os.Rename(file, newPath); err != nil {
			log.Printf("failed to move file %s: %v", file, err)
		}
	}

	err = os.RemoveAll(filepath.Join(s.OutputDir, source))
	if err != nil {
		return fmt.Errorf("failed to remove folder: %w", err)
	}

	return nil
}

// If there are any image files that end in a query (e.g filename.png?lossless=1) and the base name (e.g. filename.png) does not exist, copy the file and rename it with the base name

func (s *Service) FixQueryPaths() error {
	dir := s.OutputDir
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		fileName := d.Name()
		for _, q := range s.QueryFilePaths {

			if strings.HasSuffix(fileName, q) {
				basePath := strings.TrimSuffix(path, q)
				if _, err := os.Stat(basePath); os.IsNotExist(err) {
					err := copyFile(path, basePath)
					if err != nil {
						return fmt.Errorf("failed to copy file: %w", err)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to fix local images: %w", err)
	}
	return nil
}

func copyFile(src, dest string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return nil
}

func removeContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dir, name))
		if err != nil {
			return err
		}
	}
	return nil
}
