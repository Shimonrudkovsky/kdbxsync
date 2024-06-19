package main

import (
	"fmt"
	"log"

	"keepass_sync/google_drive"
	"keepass_sync/keepass"
	"keepass_sync/keychain"
	"keepass_sync/settings"
)

func main() {
	log.SetPrefix("### ")
	credentials := "client_credentials.json"
	keychainAccess := keychain.KeychainAccess{
		Service:     "keepass_scripts",
		Account:     "keepass_sync",
		Label:       "keepass_sync",
		AccessGroup: "424242.group.com.example",
	}

	dbSettings, err := settings.NewDatabaseSetting(keychainAccess)
	if err != nil {
		log.Fatalf("Unable to initialize settings: %v", err)
	}

	googleDriveController, err := google_drive.NewGoogleDriveController(credentials)
	if err != nil {
		log.Fatalf("Unable initialize google drive: %v", err)
	}

	// backup first everything else later :)
	err = keepass.BackupLocalKeepassDB(dbSettings)
	if err != nil {
		log.Fatalf("Unable to create backup: %v", err)
	}

	err = googleDriveController.BackupDBFile(dbSettings)
	if err != nil {
		log.Fatalf("Unable to backup remote base: %v", err)
	}

	keepasSync, err := keepass.InitKeepassDBs(dbSettings, googleDriveController)
	if err != nil {
		log.Fatalf("Unable to open one of Keepass DBs: %v", err)
	}

	err = keepasSync.SyncBases()
	if err != nil {
		log.Fatalf("Unable to sync bases: %v", err)
	}

	err = keepasSync.CleanLocal()
	if err != nil {
		log.Fatalf("Unable to clean tmp files: %v", err)
	}

	err = googleDriveController.UpdateDBFile(dbSettings)
	if err != nil {
		log.Fatalf("Unable to upload Keepass DB file to google drive: %v", err)
	}

	fmt.Print("Done")
}
