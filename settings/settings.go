package settings

import (
	"fmt"
	"keepass_sync/keychain"
	"os"
)

type DataBaseSettings struct {
	Directory        string
	FileName         string
	Password         string
	RemoteCopyPrefix string
	SyncDBName       string
	BackupDirectory  string
}

func (dbSettings *DataBaseSettings) FullFilePath() string {
	return fmt.Sprintf("%s/%s", dbSettings.Directory, dbSettings.FileName)
}

func (dbSettings *DataBaseSettings) FullRemoteCopyFilePath() string {
	return fmt.Sprintf("%s/%s_%s", dbSettings.Directory, dbSettings.RemoteCopyPrefix, dbSettings.FileName)
}

func (dbSettings *DataBaseSettings) FullSyncFilePath() string {
	return fmt.Sprintf("%s/%s", dbSettings.Directory, dbSettings.SyncDBName)
}

func getSysEnv(sysEnvName string) (string, error) {
	sysEnv := os.Getenv(sysEnvName)
	if sysEnv == "" {
		return "", fmt.Errorf("sysenv %s  not found or empty", sysEnvName)
	}
	return sysEnv, nil
}

func NewDatabaseSetting(keychainAccess keychain.KeychainAccess) (*DataBaseSettings, error) {
	pass, err := keychainAccess.GetPassword()
	if err != nil {
		return nil, fmt.Errorf("can't get keepass db password: %v", err)
	}

	directory := os.Getenv("KEEPASS_DB_DIRECTORY")
	dbFileName := os.Getenv("KEEPASS_DB_FILE_NAME")
	if directory == "" {
		return nil, fmt.Errorf("can't find KEEPASS_DB_DIRECTORY variable")
	}
	if dbFileName == "" {
		return nil, fmt.Errorf("can't find KEEPASS_DB_FILE_NAME variable")
	}

	dbSettings := DataBaseSettings{
		Directory:        directory,
		FileName:         dbFileName,
		Password:         pass,
		RemoteCopyPrefix: "remote_copy",
		SyncDBName:       "tmp.kdbx",
		BackupDirectory:  fmt.Sprintf("%s/backups", directory),
	}

	return &dbSettings, nil
}
