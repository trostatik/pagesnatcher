package pagesnatcher

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) CreateZip() error {
	err := zipDirectory(s.OutputDir, s.ZipPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	return nil
}

func zipDirectory(source, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("failed to create zip file: %w", err)
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	err = filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath := strings.TrimPrefix(path, filepath.Dir(source)+"/")

		// If it's a subdirectory, create a folder entry
		if info.IsDir() {
			_, err := zipWriter.Create(relPath + "/")
			return err
		}

		// Add a file to the zip archive
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Create a file entry in the zip archive
		writer, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		// Copy the file data to the zip entry
		_, err = io.Copy(writer, file)
		return err
	})

	if err != nil {
		return fmt.Errorf("failed to zip directory: %w", err)
	}

	return nil
}
