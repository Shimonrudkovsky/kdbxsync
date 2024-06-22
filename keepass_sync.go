package main

import (
	"fmt"
	"log"

	"keepass_sync/http"
	"keepass_sync/keepass"
	"keepass_sync/keychain"
	"keepass_sync/settings"
	"keepass_sync/storage"
)

type app struct {
	keepass *keepass.KeepassDBSync
}

func initApp(credentials string) (*app, error) {
	channel := make(chan string)
	httpServer := http.HttpServer{Channel: channel}
	keychainAccess, err := keychain.NewKeychainAccess("keychain.json")
	if err != nil {
		return nil, err
	}
	appSetting, err := settings.InitAppSettings(keychainAccess, &httpServer, credentials)
	if err != nil {
		return nil, err
	}
	storage, err := storage.NewStorage(appSetting)
	if err != nil {
		return nil, err
	}
	keepass_sync, err := keepass.InitKeepassDBSync(appSetting, storage)
	if err != nil {
		return nil, err
	}

	return &app{keepass: keepass_sync}, nil
}

func main() {
	log.SetPrefix("### ")
	credentials := "client_credentials.json"

	app, err := initApp(credentials)
	if err != nil {
		log.Fatalf("Unable to initialize application: %v", err)
	}

	keepass_sync := app.keepass
	err = keepass_sync.Backup()
	if err != nil {
		log.Fatalf("Unable to backup remote base: %v", err)
	}

	err = keepass_sync.Sync()
	if err != nil {
		log.Fatalf("Unable to sync keepass bases: %v", err)
	}

	fmt.Print("Done")
}
