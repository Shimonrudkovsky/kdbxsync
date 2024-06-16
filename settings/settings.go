package settings

import (
	"fmt"
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
