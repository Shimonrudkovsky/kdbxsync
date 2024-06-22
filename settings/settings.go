package settings

import (
	"fmt"
	"keepass_sync/http"
	"keepass_sync/keychain"
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

func getSysEnv(sysEnvName string) (string, error) {
	sysEnv := os.Getenv(sysEnvName)
	if sysEnv == "" {
		return "", fmt.Errorf("sysenv %s  not found or empty", sysEnvName)
	}
	return sysEnv, nil
}

func NewDatabaseSetting(keychainAccess keychain.KeychainAccess, httpServer *http.HttpServer) (*DataBaseSettings, error) {
	pass, err := keychainAccess.GetPassword(httpServer)
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

type AppSettings struct {
	HttpServer         *http.HttpServer
	DatabaseSettings   *DataBaseSettings
	StorageCredentials string
}

func InitAppSettings(keychainAccess *keychain.KeychainAccess, httpServer *http.HttpServer, storageCredentials string) (*AppSettings, error) {
	appSettings := AppSettings{
		HttpServer:         httpServer,
		StorageCredentials: storageCredentials,
	}
	pass, err := keychainAccess.GetPassword(httpServer)
	if err != nil {
		return nil, fmt.Errorf("can't get keepass db password: %v", err)
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
