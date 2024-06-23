package main

import (
	"log"

	"kdbxsync/http"
	"kdbxsync/keepass"
	"kdbxsync/keychain"
	"kdbxsync/settings"
	"kdbxsync/storage"
)

type app struct {
	keepass *keepass.DBSync
}

func initApp(credentials string, hhtpServerPort uint16, keychainAccessPath string) (*app, error) {
	httpServer := http.NewHTTPServer(hhtpServerPort)
	keychainAccess, err := keychain.NewKeychainAccess(keychainAccessPath)
	if err != nil {
		return nil, err
	}
	appSetting, err := settings.InitAppSettings(keychainAccess, httpServer, credentials)
	if err != nil {
		return nil, err
	}
	storage, err := storage.NewStorage(appSetting)
	if err != nil {
		return nil, err
	}
	keepassSync, err := keepass.InitKeepassDBSync(appSetting, storage)
	if err != nil {
		return nil, err
	}

	return &app{keepass: keepassSync}, nil
}

func main() {
	log.SetPrefix("### ")
	credentials := "client_credentials.json"

	app, err := initApp(credentials, 3030, "keychain.json")
	if err != nil {
		log.Fatalf("Unable to initialize application: %v", err)
	}

	keepassSync := app.keepass
	err = keepassSync.Backup()
	if err != nil {
		log.Fatalf("Unable to backup remote base: %v", err)
	}

	err = keepassSync.Sync()
	if err != nil {
		log.Fatalf("Unable to sync keepass bases: %v", err)
	}

	log.Print("Done")
}
