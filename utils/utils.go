package utils

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"

	"keepass_sync/settings"
)

func CompareFileCheckSums(filePath1 string, filePath2 string) (bool, error) {
	f1, err := os.Open(filePath1)
	if err != nil {
		return false, fmt.Errorf("can't open file: %v", err)
	}
	f2, err := os.Open(filePath2)
	if err != nil {
		return false, fmt.Errorf("can't open file: %v", err)
	}
	defer f1.Close()
	defer f2.Close()

	h1 := sha256.New()
	h2 := sha256.New()

	_, err = io.Copy(h1, f1)
	if err != nil {
		return false, fmt.Errorf("can't copy file data: %v", err)
	}
	_, err = io.Copy(h2, f2)
	if err != nil {
		return false, fmt.Errorf("can't copy file data: %v", err)
	}

	return string(h1.Sum(nil)[:]) == string(h2.Sum(nil)[:]), nil
}

func GetLatestBackup(dbSettings *settings.DataBaseSettings) (os.DirEntry, error) {
	fileList, err := os.ReadDir(dbSettings.BackupDirectory)
	if err != nil {
		return nil, fmt.Errorf("can't read directory: %v", err)
	}

	latestFileIdx := -1
	for i, file := range fileList {
		if strings.HasPrefix(file.Name(), ".") {
			continue
		}
		if latestFileIdx == -1 {
			latestFileIdx = i
		} else {
			fileInfo, _ := file.Info()
			latestFileInfo, _ := fileList[latestFileIdx].Info()

			if fileInfo.IsDir() || !fileInfo.Mode().IsRegular() {
				continue
			}
			if fileInfo.ModTime().After(latestFileInfo.ModTime()) {
				latestFileIdx = i
			}
		}
	}
	if latestFileIdx == -1 {
		return nil, fmt.Errorf("can't find the latest file")
	}

	return fileList[latestFileIdx], nil
}
