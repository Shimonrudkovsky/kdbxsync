package settings

import (
	"errors"
	"fmt"
	"kdbxsync/http"
	"kdbxsync/keychain"
	"os"
)

type EnvVars struct {
	directory  string
	dbFileName string
}

func getEnvs(directoryEnv string, dbFileNameEnv string) (*EnvVars, error) {
	directory := os.Getenv(directoryEnv)
	dbFileName := os.Getenv(dbFileNameEnv)
	if directory == "" {
		return nil, fmt.Errorf("can't find %s variable", directoryEnv)
	}
	if dbFileName == "" {
		return nil, fmt.Errorf("can't find %s variable", dbFileName)
	}

	return &EnvVars{directory: directory, dbFileName: dbFileName}, nil
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
	keychainAccess keychain.Access,
	httpServer *http.Server,
) (*DataBaseSettings, error) {
	pass, err := keychainAccess.GetPassword(httpServer)
	if err != nil {
		return nil, fmt.Errorf("can't get keepass db password: %w", err)
	}

	directory := os.Getenv("KEEPASS_DB_DIRECTORY")
	dbFileName := os.Getenv("KEEPASS_DB_FILE_NAME")
	if directory == "" {
		return nil, errors.New("can't find KEEPASS_DB_DIRECTORY variable")
	}
	if dbFileName == "" {
		return nil, errors.New("can't find KEEPASS_DB_FILE_NAME variable")
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

type AppSettings struct {
	HTTPServer         *http.Server
	DatabaseSettings   *DataBaseSettings
	StorageCredentials string
}

func InitAppSettings(
	keychainAccess *keychain.Access,
	httpServer *http.Server,
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

	envVars, err := getEnvs("KEEPASS_DB_DIRECTORY", "KEEPASS_DB_FILE_NAME")
	if err != nil {
		return nil, err
	}

	appSettings.DatabaseSettings = &DataBaseSettings{
		Directory:        envVars.directory,
		FileName:         envVars.dbFileName,
		Password:         pass,
		RemoteCopyPrefix: "remote_copy",
		SyncDBName:       "tmp.kdbx",
		BackupDirectory:  fmt.Sprintf("%s/backups", envVars.directory),
	}

	return &appSettings, nil
}
