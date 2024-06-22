package storage

import (
	"keepass_sync/settings"
)

type Storage struct {
	Settings *settings.AppSettings
	Service  *googleDriveController
}

func (storage *Storage) UpdateDBFile() error {
	err := storage.Service.UpdateDBFile()
	if err != nil {
		return err
	}
	return nil
}

func (storage *Storage) DownloadRemoteKeepassDB() error {
	err := storage.Service.DownloadRemoteKeepassDB()
	if err != nil {
		return err
	}
	return nil
}

func (storage *Storage) BackupDBFile() error {
	err := storage.Service.BackupDBFile()
	if err != nil {
		return err
	}
	return nil
}

func NewStorage(settings *settings.AppSettings) (*Storage, error) {
	googleStorage, err := newGoogleDriveController(settings)
	if err != nil {
		return nil, err
	}

	return &Storage{Settings: settings, Service: googleStorage}, nil
}
