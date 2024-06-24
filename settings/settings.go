package settings

import (
	"errors"
	"fmt"
	"kdbxsync/http"
	"kdbxsync/keychain"
	"os"
)

type EnvVars struct {
	Directory  string
	DBFileName string
}

func GetEnvs(directoryEnv string, dbFileNameEnv string) (*EnvVars, error) {
	directory := os.Getenv(directoryEnv)
	dbFileName := os.Getenv(dbFileNameEnv)
	if directory == "" {
		return nil, errors.New("can't find directory variable")
	}
	if dbFileName == "" {
		return nil, errors.New("can't find db file name variable")
	}

	return &EnvVars{Directory: directory, DBFileName: dbFileName}, nil
}

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

func NewDatabaseSetting(
	keychainAccess keychain.KeyStorage,
	httpServer http.HTTPServer,
) (*DataBaseSettings, error) {
	pass, err := keychainAccess.GetPassword(httpServer)
	if err != nil {
		return nil, fmt.Errorf("can't get keepass db password: %w", err)
	}

	envVars, err := GetEnvs("KEEPASS_DB_DIRECTORY", "KEEPASS_DB_FILE_NAME")
	if err != nil {
		return nil, err
	}

	dbSettings := DataBaseSettings{
		Directory:        envVars.Directory,
		FileName:         envVars.DBFileName,
		Password:         pass,
		RemoteCopyPrefix: "remote_copy",
		SyncDBName:       "tmp.kdbx",
		BackupDirectory:  fmt.Sprintf("%s/backups", envVars.Directory),
	}

	return &dbSettings, nil
}

type AppSettings struct {
	HTTPServer         http.HTTPServer
	DatabaseSettings   *DataBaseSettings
	StorageCredentials string
}

func InitAppSettings(
	keychainAccess *keychain.Access,
	httpServer http.HTTPServer,
	storageCredentials string,
) (*AppSettings, error) {
	appSettings := AppSettings{
		HTTPServer:         httpServer,
		StorageCredentials: storageCredentials,
	}
	pass, err := keychainAccess.GetPassword(httpServer)
	if err != nil {
		return nil, fmt.Errorf("can't get keepass db password: %w", err)
	}

	envVars, err := GetEnvs("KEEPASS_DB_DIRECTORY", "KEEPASS_DB_FILE_NAME")
	if err != nil {
		return nil, err
	}

	appSettings.DatabaseSettings = &DataBaseSettings{
		Directory:        envVars.Directory,
		FileName:         envVars.DBFileName,
		Password:         pass,
		RemoteCopyPrefix: "remote_copy",
		SyncDBName:       "tmp.kdbx",
		BackupDirectory:  fmt.Sprintf("%s/backups", envVars.Directory),
	}

	return &appSettings, nil
}
