package keepass

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"kdbxsync/settings"

	"github.com/tobischo/gokeepasslib/v3"
)

type Storage interface {
	UpdateDBFile() error
	DownloadRemoteKeepassDB() error
	BackupDBFile() error
}

type DBSync struct {
	localKeepassDB      *gokeepasslib.Database
	remoteKeepassDBCopy *gokeepasslib.Database
	syncKeepassDB       *gokeepasslib.Database
	storage             Storage
	settings            *settings.AppSettings
}

func NewKeepassDBSync(
	localDBFileObj io.Reader,
	remoteDBCopyFileObj io.Reader,
	tmpSyncFileObj io.Reader,
	storage Storage,
	settings *settings.AppSettings,
) (*DBSync, error) {
	// new db instances to decode files into
	localDB := gokeepasslib.NewDatabase()
	remoteDBCopy := gokeepasslib.NewDatabase()
	syncDB := gokeepasslib.NewDatabase()

	cred := gokeepasslib.NewPasswordCredentials(settings.DatabaseSettings.Password)
	localDB.Credentials = cred
	remoteDBCopy.Credentials = cred
	syncDB.Credentials = cred

	// decoding local remote copy and tmp bases
	err := gokeepasslib.NewDecoder(localDBFileObj).Decode(localDB)
	if err != nil {
		return nil, fmt.Errorf("can't initialize local Keepass DB: %w", err)
	}
	err = gokeepasslib.NewDecoder(remoteDBCopyFileObj).Decode(remoteDBCopy)
	if err != nil {
		return nil, fmt.Errorf("can't initialize remote Keepass DB copy: %w", err)
	}
	err = gokeepasslib.NewDecoder(tmpSyncFileObj).Decode(syncDB)
	if err != nil {
		return nil, fmt.Errorf("can't initialize tmp sync Keepass DB: %w", err)
	}

	err = localDB.UnlockProtectedEntries()
	defer localDB.LockProtectedEntries()
	if err != nil {
		return nil, fmt.Errorf("can't unlock protected entries: %w", err)
	}
	err = remoteDBCopy.UnlockProtectedEntries()
	defer remoteDBCopy.LockProtectedEntries()
	if err != nil {
		return nil, fmt.Errorf("can't unlock protected entries: %w", err)
	}
	err = syncDB.UnlockProtectedEntries()
	defer syncDB.LockProtectedEntries()
	if err != nil {
		return nil, fmt.Errorf("can't unlock protected entries: %w", err)
	}

	return &DBSync{
		localKeepassDB:      localDB,
		remoteKeepassDBCopy: remoteDBCopy,
		syncKeepassDB:       syncDB,
		settings:            settings,
		storage:             storage,
	}, nil
}

func (keepassDBSync *DBSync) SaveSyncDB() error {
	syncDBFileObj, err := os.OpenFile(
		keepassDBSync.settings.DatabaseSettings.FullSyncFilePath(),
		os.O_WRONLY,
		os.ModeAppend,
	)
	if err != nil {
		return fmt.Errorf("can't open sync DB file: %w", err)
	}
	defer syncDBFileObj.Close()

	err = keepassDBSync.syncKeepassDB.LockProtectedEntries()
	if err != nil {
		return err
	}
	keepassEncoder := gokeepasslib.NewEncoder(syncDBFileObj)
	if err = keepassEncoder.Encode(keepassDBSync.syncKeepassDB); err != nil {
		return fmt.Errorf("can't encode sync keepass DB: %w", err)
	}
	err = syncDBFileObj.Sync()
	if err != nil {
		return err
	}

	return nil
}

func (keepassDBSync *DBSync) syncBases() error {
	localEntries := keepassDBSync.localKeepassDB.Content.Root.Groups[0].Entries
	remoteCopyEntries := keepassDBSync.remoteKeepassDBCopy.Content.Root.Groups[0].Entries

	var missingInLocal []gokeepasslib.Entry
	var missingInRemote []gokeepasslib.Entry
	var newEntries []gokeepasslib.Entry

	// convert entries lists to maps to ease search by uuid
	mapLocalEntries := make(map[string]gokeepasslib.Entry)
	mapRemoteEntries := make(map[string]gokeepasslib.Entry)
	for _, entry := range localEntries {
		mapLocalEntries[fmt.Sprintf("%x", entry.UUID)] = entry
	}
	for _, entry := range remoteCopyEntries {
		mapRemoteEntries[fmt.Sprintf("%x", entry.UUID)] = entry
	}

	// find missing entries in remote Keepass DB and for matching uuids find the latest modified version
	for localKey, localValue := range mapLocalEntries {
		remoteValue, ok := mapRemoteEntries[localKey]
		if !ok {
			missingInRemote = append(missingInRemote, localValue)
		} else {
			// TODO: need to check if LastModificationTime could be nil
			if remoteValue.Times.LastModificationTime.Time.After(localValue.Times.LastModificationTime.Time) {
				newEntries = append(newEntries, remoteValue)
			} else {
				newEntries = append(newEntries, localValue)
			}
		}
	}
	// find missing entries in local Keepass DB
	for remoteKey, remoteValue := range mapRemoteEntries {
		_, ok := mapLocalEntries[remoteKey]
		if !ok {
			missingInLocal = append(missingInLocal, remoteValue)
		}
	}
	// add missing entries
	newEntries = append(newEntries, missingInLocal...)
	newEntries = append(newEntries, missingInRemote...)
	keepassDBSync.syncKeepassDB.Content.Root.Groups[0].Entries = newEntries

	err := keepassDBSync.SaveSyncDB()
	if err != nil {
		return fmt.Errorf("can't save new keepas DB: %w", err)
	}

	return nil
}

func (keepassDBSync *DBSync) cleanLocal() error {
	// check the recent backup first
	// remove remote db copy
	// remove original local db file
	// rename tmp sync db file to original name

	latestBackup, err := GetLatestBackup(keepassDBSync.settings.DatabaseSettings)
	if err != nil {
		return err
	}

	// checking checksum of the latest backup
	isCheckSumsEqual, err := CompareFileCheckSums(
		keepassDBSync.settings.DatabaseSettings.FullFilePath(),
		fmt.Sprintf("%s/%s", keepassDBSync.settings.DatabaseSettings.BackupDirectory, latestBackup.Name()),
	)
	if err != nil {
		return fmt.Errorf("can't compare hashes: %w", err)
	}

	if !isCheckSumsEqual {
		return errors.New("can't find latest backup")
	}

	// deleting remote db copy
	err = os.Remove(keepassDBSync.settings.DatabaseSettings.FullRemoteCopyFilePath())
	if err != nil {
		return fmt.Errorf("can't remove remote copy: %w", err)
	}
	// deleting original db file
	err = os.Remove(keepassDBSync.settings.DatabaseSettings.FullFilePath())
	if err != nil {
		return fmt.Errorf("can't delete local db file: %w", err)
	}
	// renaming tmp sync file as original db file
	err = os.Rename(
		keepassDBSync.settings.DatabaseSettings.FullSyncFilePath(),
		keepassDBSync.settings.DatabaseSettings.FullFilePath(),
	)
	if err != nil {
		return fmt.Errorf("can't rename tmp sync file: %w", err)
	}

	return nil
}

func (keepassDBSync *DBSync) Sync() error {
	err := keepassDBSync.syncBases()
	if err != nil {
		return err
	}
	err = keepassDBSync.cleanLocal()
	if err != nil {
		return err
	}
	err = keepassDBSync.storage.UpdateDBFile()
	if err != nil {
		return err
	}

	return nil
}

func (keepassDBSync *DBSync) Backup() error {
	err := backupLocalKeepassDB(keepassDBSync.settings.DatabaseSettings)
	if err != nil {
		return fmt.Errorf("can't create backup: %w", err)
	}
	err = keepassDBSync.storage.BackupDBFile()
	if err != nil {
		return fmt.Errorf("can't backup remote base: %w", err)
	}

	return nil
}

func backupLocalKeepassDB(dbSettings *settings.DataBaseSettings) error {
	nowTimeStamp := time.Now()
	dbFilePath := dbSettings.FullFilePath()

	info, err := os.Stat(dbFilePath)
	if err != nil {
		return fmt.Errorf("can't get local Keepass BD file info: %w", err)
	}
	data, err := os.ReadFile(dbFilePath)
	if err != nil {
		return fmt.Errorf("can't read local Keepass DB file: %w", err)
	}

	err = os.MkdirAll(dbSettings.BackupDirectory, 0700)
	if err != nil {
		return fmt.Errorf("can't create backup directory: %w", err)
	}
	backupFilePath := fmt.Sprintf(
		"%s/%s-%s",
		dbSettings.BackupDirectory,
		nowTimeStamp.Format("2006-01-02T15-04-05"),
		dbSettings.FileName,
	)
	err = os.WriteFile(backupFilePath, data, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("can't write a backup file: %w", err)
	}

	return nil
}

func InitKeepassDBSync(settings *settings.AppSettings, storage Storage) (*DBSync, error) {
	localKeepassDBPath := settings.DatabaseSettings.FullFilePath()

	localKeepassDBObj, err := os.Open(localKeepassDBPath)
	if err != nil {
		return nil, fmt.Errorf("can't open local Keepass DB file: %w", err)
	}
	defer localKeepassDBObj.Close()

	err = storage.DownloadRemoteKeepassDB()
	if err != nil {
		return nil, fmt.Errorf("can't download remote Keepass DB file: %w", err)
	}

	remoteDBCopyObj, err := os.Open(settings.DatabaseSettings.FullRemoteCopyFilePath())
	if err != nil {
		return nil, fmt.Errorf("can't open remote DB copy: %w", err)
	}
	defer remoteDBCopyObj.Close()

	syncDBObj, err := os.Create(settings.DatabaseSettings.FullSyncFilePath())
	if err != nil {
		return nil, fmt.Errorf("can't create sync db file: %w", err)
	}
	defer syncDBObj.Close()
	_, err = io.Copy(syncDBObj, localKeepassDBObj)
	if err != nil {
		return nil, fmt.Errorf("can't copy local db file to sync db file: %w", err)
	}
	err = syncDBObj.Sync()
	if err != nil {
		return nil, err
	}
	_, err = syncDBObj.Seek(0, 0)
	if err != nil {
		return nil, err
	}
	_, err = localKeepassDBObj.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	keepasSync, err := NewKeepassDBSync(localKeepassDBObj, remoteDBCopyObj, syncDBObj, storage, settings)
	if err != nil {
		return nil, fmt.Errorf("can't open one of Keepass DBs: %w", err)
	}

	return keepasSync, nil
}

func CompareFileCheckSums(filePath1 string, filePath2 string) (bool, error) {
	f1, err := os.Open(filePath1)
	if err != nil {
		return false, fmt.Errorf("can't open file: %w", err)
	}
	f2, err := os.Open(filePath2)
	if err != nil {
		return false, fmt.Errorf("can't open file: %w", err)
	}
	defer f1.Close()
	defer f2.Close()

	h1 := sha256.New()
	h2 := sha256.New()

	_, err = io.Copy(h1, f1)
	if err != nil {
		return false, fmt.Errorf("can't copy file data: %w", err)
	}
	_, err = io.Copy(h2, f2)
	if err != nil {
		return false, fmt.Errorf("can't copy file data: %w", err)
	}

	return string(h1.Sum(nil)[:]) == string(h2.Sum(nil)[:]), nil
}

func GetLatestBackup(dbSettings *settings.DataBaseSettings) (os.DirEntry, error) {
	fileList, err := os.ReadDir(dbSettings.BackupDirectory)
	if err != nil {
		return nil, fmt.Errorf("can't read directory: %w", err)
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
		return nil, errors.New("can't find the latest file")
	}

	return fileList[latestFileIdx], nil
}
