package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"keepass_sync/google_drive"
	"keepass_sync/keepass"
	"keepass_sync/settings"
)

func compareFileCheckSums(filePath1 string, filePath2 string) (bool, error) {
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

func getLatestBackup(dbSettings *settings.DataBaseSettings) (os.DirEntry, error) {
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

func cleanLocal(dbSettings *settings.DataBaseSettings) error {
	// check the recent backup first
	// remove remote db copy
	// remove original local db file
	// rename tmp sync db file to original name

	latestBackup, err := getLatestBackup(dbSettings)
	if err != nil {
		return err
	}

	// checking checksum of the latest backup
	isCheckSumsEqual, err := compareFileCheckSums(
		dbSettings.FullFilePath(),
		fmt.Sprintf("%s/%s", dbSettings.BackupDirectory, latestBackup.Name()),
	)
	if err != nil {
		return fmt.Errorf("can't compare hashes: %v", err)
	}

	if !isCheckSumsEqual {
		return fmt.Errorf("can't find latest backup")
	}

	// deleting remote db copy
	err = os.Remove(dbSettings.FullRemoteCopyFilePath())
	if err != nil {
		return fmt.Errorf("can't remove remote copy: %v", err)
	}
	// deleting original db file
	err = os.Remove(dbSettings.FullFilePath())
	if err != nil {
		return fmt.Errorf("can't delete local db file: %v", err)
	}
	// renaming tmp sync file as original db file
	err = os.Rename(dbSettings.FullSyncFilePath(), dbSettings.FullFilePath())
	if err != nil {
		return fmt.Errorf("can't rename tmp sync file: %v", err)
	}

	return nil
}

func main() {
	directory := "/Users/semen-rudkovskiy/Documents/Keepass"
	// TODO: remove openly stored passwords
	dbSettings := settings.DataBaseSettings{
		Directory:        directory,
		FileName:         "test1.kdbx",
		Password:         "1",
		RemoteCopyPrefix: "remote_copy",
		SyncDBName:       "tmp.kdbx",
		BackupDirectory:  fmt.Sprintf("%s/backups", directory),
	}
	log.SetPrefix("### ")
	credentials := "client_credentials.json"

	googleDriveController, err := google_drive.NewGoogleDriveController(credentials)
	if err != nil {
		log.Fatalf("Unable initialize google drive: %v", err)
	}

	// backup first everything else later :)
	err = keepass.BackupLocalKeepassDB(&dbSettings)
	if err != nil {
		log.Fatalf("Unable to create backup: %v", err)
	}

	err = googleDriveController.BackupDBFile(&dbSettings)
	if err != nil {
		log.Fatalf("Unable to backup remote base: %v", err)
	}

	keepasSync, err := keepass.InitKeepasDBs(&dbSettings, googleDriveController)
	if err != nil {
		log.Fatalf("Unable to open one of Keepass DBs: %v", err)
	}

	err = keepasSync.SyncBases(&dbSettings)
	if err != nil {
		log.Fatalf("Unable to sync bases: %v", err)
	}

	err = cleanLocal(&dbSettings)
	if err != nil {
		log.Fatalf("Unable to clean tmp files: %v", err)
	}

	err = googleDriveController.UpdateDBFile(&dbSettings)
	if err != nil {
		log.Fatalf("Unable to upload Keepass DB file to google drive: %v", err)
	}

	//############### print-debug part :)
	l, _ := googleDriveController.Find("Backups")
	s, _ := json.MarshalIndent(l, "", "    ")
	fmt.Println(string(s))
	localEntries := keepasSync.LocalKeepassDB.Content.Root.Groups[0].Entries
	remoteCopyEntries := keepasSync.RemoteKeepassDBCopy.Content.Root.Groups[0].Entries
	fmt.Println("Local DB:")
	fmt.Printf("UUID: %x\n", localEntries[0].UUID)
	fmt.Printf("Time: %v\n", localEntries[0].Times.LastModificationTime)
	fmt.Println("Remote DB copy:")
	fmt.Printf("UUID: %x\n", remoteCopyEntries[0].UUID)
	fmt.Printf("Time: %v\n", remoteCopyEntries[0].Times.LastModificationTime)
}
