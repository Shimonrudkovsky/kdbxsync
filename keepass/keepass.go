package keepass

import (
	"fmt"
	"io"
	"os"
	"time"

	"keepass_sync/google_drive"
	"keepass_sync/settings"
	"keepass_sync/utils"

	"github.com/tobischo/gokeepasslib/v3"
)

type KeepassDBSync struct {
	LocalKeepassDB      *gokeepasslib.Database
	RemoteKeepassDBCopy *gokeepasslib.Database
	SyncKeepassDB       *gokeepasslib.Database
	dbSettings          *settings.DataBaseSettings
}

func newKeepassDBSync(
	localDBFileObj *os.File,
	remoteDBCopyFileObj *os.File,
	tmpSyncFileObj *os.File,
	dbSettings *settings.DataBaseSettings,
) (*KeepassDBSync, error) {
	// new db instances to decode files into
	localDB := gokeepasslib.NewDatabase()
	remoteDBCopy := gokeepasslib.NewDatabase()
	syncDB := gokeepasslib.NewDatabase()

	// pass
	cred := gokeepasslib.NewPasswordCredentials(dbSettings.Password)
	localDB.Credentials = cred
	remoteDBCopy.Credentials = cred
	syncDB.Credentials = cred

	// decoding local and remote copy
	err := gokeepasslib.NewDecoder(localDBFileObj).Decode(localDB)
	if err != nil {
		return nil, fmt.Errorf("can't initialize local Keepass DB: %v", err)
	}
	err = gokeepasslib.NewDecoder(remoteDBCopyFileObj).Decode(remoteDBCopy)
	if err != nil {
		return nil, fmt.Errorf("can't initialize remote Keepass DB copy: %v", err)
	}
	err = gokeepasslib.NewDecoder(tmpSyncFileObj).Decode(syncDB)
	if err != nil {
		return nil, fmt.Errorf("can't initialize tmp sync Keepass DB: %v", err)
	}

	localDB.UnlockProtectedEntries()
	defer localDB.LockProtectedEntries()
	remoteDBCopy.UnlockProtectedEntries()
	defer remoteDBCopy.LockProtectedEntries()
	syncDB.UnlockProtectedEntries()
	defer syncDB.LockProtectedEntries()

	return &KeepassDBSync{
		LocalKeepassDB:      localDB,
		RemoteKeepassDBCopy: remoteDBCopy,
		SyncKeepassDB:       syncDB,
		dbSettings:          dbSettings,
	}, nil
}

func (keepassSync KeepassDBSync) SaveSyncDB() error {
	syncDBFileObj, err := os.OpenFile(keepassSync.dbSettings.FullSyncFilePath(), os.O_WRONLY, os.ModeAppend)
	if err != nil {
		return fmt.Errorf("can't open sync DB file: %v", err)
	}
	defer syncDBFileObj.Close()

	keepassSync.SyncKeepassDB.LockProtectedEntries()
	keepassEncoder := gokeepasslib.NewEncoder(syncDBFileObj)
	if err := keepassEncoder.Encode(keepassSync.SyncKeepassDB); err != nil {
		return fmt.Errorf("can't encode sync keepass DB: %v", err)
	}
	syncDBFileObj.Sync()

	return nil
}

func (keepasDBSync KeepassDBSync) SyncBases() error {
	localEntries := keepasDBSync.LocalKeepassDB.Content.Root.Groups[0].Entries
	remoteCopyEntries := keepasDBSync.RemoteKeepassDBCopy.Content.Root.Groups[0].Entries

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
	keepasDBSync.SyncKeepassDB.Content.Root.Groups[0].Entries = newEntries

	err := keepasDBSync.SaveSyncDB()
	if err != nil {
		return fmt.Errorf("can't save new keepas DB: %v", err)
	}

	return nil
}

func (keepasDBSync KeepassDBSync) CleanLocal() error {
	// check the recent backup first
	// remove remote db copy
	// remove original local db file
	// rename tmp sync db file to original name

	latestBackup, err := utils.GetLatestBackup(keepasDBSync.dbSettings)
	if err != nil {
		return err
	}

	// checking checksum of the latest backup
	isCheckSumsEqual, err := utils.CompareFileCheckSums(
		keepasDBSync.dbSettings.FullFilePath(),
		fmt.Sprintf("%s/%s", keepasDBSync.dbSettings.BackupDirectory, latestBackup.Name()),
	)
	if err != nil {
		return fmt.Errorf("can't compare hashes: %v", err)
	}

	if !isCheckSumsEqual {
		return fmt.Errorf("can't find latest backup")
	}

	// deleting remote db copy
	err = os.Remove(keepasDBSync.dbSettings.FullRemoteCopyFilePath())
	if err != nil {
		return fmt.Errorf("can't remove remote copy: %v", err)
	}
	// deleting original db file
	err = os.Remove(keepasDBSync.dbSettings.FullFilePath())
	if err != nil {
		return fmt.Errorf("can't delete local db file: %v", err)
	}
	// renaming tmp sync file as original db file
	err = os.Rename(keepasDBSync.dbSettings.FullSyncFilePath(), keepasDBSync.dbSettings.FullFilePath())
	if err != nil {
		return fmt.Errorf("can't rename tmp sync file: %v", err)
	}

	return nil
}

func BackupLocalKeepassDB(dbSettings *settings.DataBaseSettings) error {
	nowTimeStamp := time.Now()
	dbFilePath := dbSettings.FullFilePath()

	info, err := os.Stat(dbFilePath)
	if err != nil {
		return fmt.Errorf("can't get local Keepass BD file info: %v", err)
	}
	data, err := os.ReadFile(dbFilePath)
	if err != nil {
		return fmt.Errorf("can't read local Keepass DB file: %v", err)
	}

	err = os.MkdirAll(dbSettings.BackupDirectory, 0700)
	if err != nil {
		return fmt.Errorf("can't create backup directory: %v", err)
	}
	err = os.WriteFile(fmt.Sprintf("%s/%s-%s", dbSettings.BackupDirectory, nowTimeStamp.Format("2006-01-02T15-04-05"), dbSettings.FileName), data, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("can't write a backup file: %v", err)
	}

	return nil
}

func InitKeepassDBs(
	dbSettings *settings.DataBaseSettings,
	googleDriveController *google_drive.GoogleDriveController,
) (*KeepassDBSync, error) {
	localKeepassDBPath := dbSettings.FullFilePath()

	localKeepassDBObj, err := os.Open(localKeepassDBPath)
	if err != nil {
		return nil, fmt.Errorf("can't open local Keepass DB file: %v", err)
	}
	defer localKeepassDBObj.Close()

	err = googleDriveController.DownloadRemoteKeepassDB(dbSettings)
	if err != nil {
		return nil, fmt.Errorf("can't download remote Keepass DB file: %v", err)
	}

	remoteDBCopyObj, err := os.Open(dbSettings.FullRemoteCopyFilePath())
	if err != nil {
		return nil, fmt.Errorf("can't open remote DB copy: %v", err)
	}
	defer remoteDBCopyObj.Close()

	syncDBObj, err := os.Create(dbSettings.FullSyncFilePath())
	if err != nil {
		return nil, fmt.Errorf("can't create sync db file: %v", err)
	}
	defer syncDBObj.Close()
	_, err = io.Copy(syncDBObj, localKeepassDBObj)
	if err != nil {
		return nil, fmt.Errorf("can't copy local db file to sync db file: %v", err)
	}
	syncDBObj.Sync()
	syncDBObj.Seek(0, 0)
	localKeepassDBObj.Seek(0, 0)

	keepasSync, err := newKeepassDBSync(localKeepassDBObj, remoteDBCopyObj, syncDBObj, dbSettings)
	if err != nil {
		return nil, fmt.Errorf("can't open one of Keepass DBs: %v", err)
	}

	return keepasSync, nil
}
